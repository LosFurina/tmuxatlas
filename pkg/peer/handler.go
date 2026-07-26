package peer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024 * 16,
}

// Handler handles incoming peer WebSocket connections (hub side)
type Handler struct {
	manager          *Manager
	peerStore        *identity.PeerStore
	tracker          *toolevents.Tracker
	pairing          *identity.PairingManager
	ptyRelay         *PTYRelay
	publicURL        string
	handshakeTimeout time.Duration
}

// NewHandler creates a new peer connection handler
func NewHandler(manager *Manager, peerStore *identity.PeerStore, tracker *toolevents.Tracker, pairing *identity.PairingManager, ptyRelay *PTYRelay, publicURL string) *Handler {
	return &Handler{
		manager:          manager,
		peerStore:        peerStore,
		tracker:          tracker,
		pairing:          pairing,
		ptyRelay:         ptyRelay,
		publicURL:        publicURL,
		handshakeTimeout: 10 * time.Second,
	}
}

// HandlePeer handles the /ws/peer endpoint for control channel connections
func (h *Handler) HandlePeer(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.WithError(err).Warn("peer ws upgrade failed")
		return
	}
	defer conn.Close()
	conn.SetReadLimit(256 << 10)

	log := logrus.WithField("remote", r.RemoteAddr)

	// Step 1: Send challenge
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		log.WithError(err).Error("failed to generate challenge")
		return
	}
	challengeB64 := base64.StdEncoding.EncodeToString(challengeBytes)

	challengeMsg, _ := NewMessage(MsgChallenge, ChallengePayload{
		Challenge: challengeB64,
	})
	if err := conn.WriteJSON(challengeMsg); err != nil {
		log.WithError(err).Debug("failed to send challenge")
		return
	}

	// Step 2: Read auth response
	conn.SetReadDeadline(time.Now().Add(h.handshakeTimeout))
	var authMsg Message
	if err := conn.ReadJSON(&authMsg); err != nil {
		log.WithError(err).Debug("failed to read auth")
		return
	}
	conn.SetReadDeadline(time.Time{})

	if authMsg.Type != MsgAuth {
		sendAuthFail(conn, "expected auth message")
		return
	}

	var authPayload AuthPayload
	if err := json.Unmarshal(authMsg.Payload, &authPayload); err != nil {
		sendAuthFail(conn, "invalid auth payload")
		return
	}

	// Step 3: Verify signature against known peers
	peer := h.peerStore.GetByPublicKey(authPayload.PublicKey)
	if peer == nil {
		sendAuthFail(conn, "unknown peer")
		return
	}

	sig, err := identity.ParseSignature(authPayload.Signature)
	if err != nil {
		sendAuthFail(conn, "invalid signature encoding")
		return
	}

	if !identity.Verify(authPayload.PublicKey, challengeBytes, sig) {
		sendAuthFail(conn, "invalid signature")
		return
	}

	// Auth successful
	authOK, _ := NewMessage(MsgAuthOK, nil)
	if err := conn.WriteJSON(authOK); err != nil {
		return
	}

	peerID := peer.Fingerprint()
	log = log.WithFields(logrus.Fields{"peer": peer.Name, "id": peerID})
	log.Info("peer authenticated")

	// Runtime protocol negotiation is mandatory after identity authentication.
	conn.SetReadDeadline(time.Now().Add(h.handshakeTimeout))
	var helloMessage Message
	if err := conn.ReadJSON(&helloMessage); err != nil {
		sendRuntimeFailure(conn, ErrorProtocolIncompatible, "runtime hello required")
		return
	}
	if helloMessage.Type != MsgHello {
		sendRuntimeFailure(conn, ErrorProtocolIncompatible, "runtime hello required before activation")
		return
	}
	var hello HelloPayload
	if err := json.Unmarshal(helloMessage.Payload, &hello); err != nil {
		sendRuntimeFailure(conn, ErrorProtocolIncompatible, "invalid runtime hello")
		return
	}
	generation := h.manager.ReserveGeneration(peerID)
	helloAck, err := NegotiateHello(hello, generation, common.VERSION)
	if err != nil {
		sendRuntimeFailure(conn, ErrorProtocolIncompatible, "no compatible runtime protocol")
		return
	}
	ackMessage, _ := NewMessage(MsgHelloAck, helloAck)
	if err := conn.WriteJSON(ackMessage); err != nil {
		return
	}

	// Configure ping/pong for connection liveness
	conn.SetPingHandler(func(data string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	peerConn := newPeerConnection(
		r.Context(), peerID, generation, helloAck.Capabilities, hello.AgentInstanceID, 64,
		func(message *Message) error { return conn.WriteJSON(message) },
		conn.Close,
	)
	if !h.manager.ActivateAuthorizedPeer(peerID, peer.Name, authPayload.PublicKey, peerConn) {
		sendRuntimeFailure(conn, ErrorStaleGeneration, "newer connection already active")
		return
	}
	h.manager.UpdatePeerVersion(peerID, hello.BuildVersion)
	defer func() {
		h.manager.UnregisterPeerGeneration(peerID, generation)
		peerConn.Close()
		peerConn.Wait()
	}()
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Send current aggregated state to the new peer
	h.sendPeerState(peerConn)

	// Subscribe to state changes and forward to this peer
	stateCh := h.manager.Subscribe()
	defer h.manager.Unsubscribe(stateCh)

	go func() {
		for evt := range stateCh {
			// Don't echo a peer's own events back to it
			if evt.Host == peerID {
				continue
			}
			msg, err := NewMessage(MsgPeerState, PeerStatePayload{
				Hosts: h.manager.GetHosts(),
			})
			if err != nil {
				continue
			}
			_ = peerConn.Send(context.Background(), msg)
		}
	}()

	// Read loop: process messages from peer
	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.WithError(err).Debug("peer read error")
			}
			break
		}

		h.handlePeerMessage(peerID, peerConn, &msg, log)
	}

	log.Info("peer disconnected")
}

// handlePeerMessage dispatches a message from a connected peer
func (h *Handler) handlePeerMessage(peerID string, connection *PeerConnection, msg *Message, log *logrus.Entry) {
	if !h.manager.IsCurrent(connection) {
		log.WithField("generation", connection.Generation).Debug("ignoring message from stale generation")
		return
	}
	switch msg.Type {
	case MsgRuntimeAck:
		var ack RuntimeAck
		if json.Unmarshal(msg.Payload, &ack) == nil {
			connection.requests.Accept(ack)
		}
	case MsgRuntimeResult:
		var result RuntimeResult
		if json.Unmarshal(msg.Payload, &result) == nil {
			connection.requests.CompleteResult(result)
		}
	case MsgRuntimeError:
		var runtimeError RuntimeError
		if json.Unmarshal(msg.Payload, &runtimeError) == nil && runtimeError.RequestID != "" {
			connection.requests.CompleteError(runtimeError)
		}
	case MsgStateUpdate:
		var payload StateUpdatePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid state-update")
			return
		}
		h.manager.UpdatePeerSessions(peerID, payload.Sessions)
		if payload.Version != "" {
			h.manager.UpdatePeerVersion(peerID, payload.Version)
		}

	case MsgStateEvent:
		var payload StateEventPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid state-event")
			return
		}
		// Trigger a sessions-changed broadcast
		h.manager.UpdatePeerSessions(peerID, h.getPeerSessions(peerID))

	case MsgToolEvent:
		var payload ToolEventPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid tool-event")
			return
		}
		if payload.Event != nil {
			payload.Event.Host = peerID
			payload.Event.HostName = h.manager.GetHostName(peerID)
			log.WithFields(logrus.Fields{
				"tool":    payload.Event.Tool,
				"status":  payload.Event.Status,
				"session": payload.Event.Session,
			}).Debug("received tool event from peer")
			h.tracker.Record(payload.Event)
		}

	case MsgActivityUpdate:
		var payload ActivityUpdatePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid activity-update")
			return
		}
		// Stamp host on each snapshot
		for _, s := range payload.Snapshots {
			s.Host = peerID
		}
		h.manager.UpdatePeerActivity(peerID, payload.Snapshots)

	case MsgStats:
		var payload StatsPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid stats")
			return
		}
		h.manager.UpdatePeerStats(peerID, payload.Stats)

	default:
		log.WithField("type", msg.Type).Debug("unknown message type from peer")
	}
}

// sendPeerState sends the full aggregated state to a peer
func (h *Handler) sendPeerState(peerConn *PeerConnection) {
	msg, err := NewMessage(MsgPeerState, PeerStatePayload{
		Hosts: h.manager.GetHosts(),
	})
	if err != nil {
		return
	}
	_ = peerConn.Send(context.Background(), msg)
}

// getPeerSessions returns the current sessions for a peer (from cache)
func (h *Handler) getPeerSessions(peerID string) []*tmux.Session {
	h.manager.mu.RLock()
	defer h.manager.mu.RUnlock()
	if host, ok := h.manager.hosts[peerID]; ok {
		return host.Sessions
	}
	return nil
}

// HandlePairing handles the POST /api/pair/complete endpoint for the pairing handshake
func (h *Handler) HandlePairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
		Version   int    `json:"version"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	name, nameErr := identity.NormalizeName(req.Name)
	publicKey, keyErr := identity.ParsePublicKey(req.PublicKey)
	signature, signatureErr := identity.ParseSignature(req.Signature)
	transcript := identity.PairingTranscript(h.publicURL, req.Code, name, req.PublicKey)
	if req.Version != identity.PairingVersion || req.Code == "" || nameErr != nil || keyErr != nil ||
		signatureErr != nil || !identity.Verify(req.PublicKey, transcript, signature) || len(publicKey) == 0 {
		http.Error(w, "pairing failed", http.StatusUnauthorized)
		return
	}

	log := logrus.WithField("remote", r.RemoteAddr)

	if err := h.pairing.Complete(req.Code, func() error {
		return h.peerStore.Add(identity.Peer{
			Name: name, PublicKey: req.PublicKey, PairedAt: time.Now(),
		})
	}); err != nil {
		log.WithField("result", "rejected").Warn("pairing completion rejected")
		http.Error(w, "pairing failed", http.StatusUnauthorized)
		return
	}

	// Respond with the hub identity. TLS trust is provided by the system trust store.
	resp := map[string]string{
		"status":     "paired",
		"name":       h.manager.LocalName(),
		"public_key": h.manager.identity.PublicKey,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.WithField("peer", name).Info("peer paired successfully")
}

func sendAuthFail(conn *websocket.Conn, reason string) {
	msg, _ := NewMessage(MsgAuthFail, map[string]string{"reason": reason})
	conn.WriteJSON(msg)
}

func sendRuntimeFailure(conn *websocket.Conn, code ErrorCode, reason string) {
	msg, _ := NewMessage(MsgRuntimeError, RuntimeError{Code: code, Message: reason})
	_ = conn.WriteJSON(msg)
}
