package peer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/stats"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

// hasScheme checks if an address string starts with a known URL scheme.
func hasScheme(addr string) bool {
	for _, prefix := range []string{"ws://", "wss://", "http://", "https://"} {
		if len(addr) > len(prefix) && addr[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func hubWebSocketURL(hubURL, path string) (*url.URL, error) {
	if !hasScheme(hubURL) {
		if strings.Contains(hubURL, "://") {
			return nil, fmt.Errorf("unsupported hub URL scheme")
		}
		hubURL = "wss://" + hubURL
	}
	u, err := url.Parse(hubURL)
	if err != nil {
		return nil, fmt.Errorf("parse hub URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
	default:
		return nil, fmt.Errorf("unsupported hub URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("hub URL host is required")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("hub URL must not contain credentials, a path, query, or fragment")
	}
	u.Path = path
	return u, nil
}

// Client connects to a hub and syncs local state
type Client struct {
	hubURL      string
	identity    *identity.Identity
	peerStore   *identity.PeerStore
	localMgr    *state.Manager
	peerMgr     *Manager
	actTracker  *activity.Tracker
	toolTracker *toolevents.Tracker
	tmuxClient  *tmux.Client

	mu   sync.Mutex
	conn *websocket.Conn

	ptyManager          *PTYManager
	instanceID          string
	runtimeGeneration   uint64
	runtimeCapabilities map[string]struct{}
	outcomeCache        *OutcomeCache
	dispatcher          *RequestDispatcher
}

// NewClient creates a new peer client
func NewClient(hubURL string, id *identity.Identity, peerStore *identity.PeerStore,
	localMgr *state.Manager, peerMgr *Manager, actTracker *activity.Tracker,
	toolTracker *toolevents.Tracker, tmuxPath string) *Client {

	tmuxClient, _ := tmux.NewClient()
	c := &Client{
		hubURL:      hubURL,
		identity:    id,
		peerStore:   peerStore,
		localMgr:    localMgr,
		peerMgr:     peerMgr,
		actTracker:  actTracker,
		toolTracker: toolTracker,
		tmuxClient:  tmuxClient,
		instanceID:  randomInstanceID(),
	}
	cacheConfig, err := OutcomeCacheConfigFromEnv()
	if err != nil {
		logrus.WithError(err).Warn("invalid Peer outcome cache configuration; using defaults")
		cacheConfig = DefaultOutcomeCacheConfig()
	}
	c.outcomeCache, _ = NewOutcomeCache(cacheConfig, nil)
	c.ptyManager = NewPTYManager(tmuxPath, actTracker, c)
	return c
}

// Run connects to the hub and maintains the connection with reconnection
func (c *Client) Run(ctx context.Context) {
	log := logrus.WithField("hub", c.hubURL)

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		connStart := time.Now()
		err := c.connectAndRun(ctx)
		if err != nil {
			log.WithError(err).Warn("hub connection lost")
		}

		// Reset backoff if the connection was up for a reasonable duration
		if time.Since(connStart) > 30*time.Second {
			backoff = time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Client) connectAndRun(ctx context.Context) error {
	log := logrus.WithField("hub", c.hubURL)

	// Build WebSocket URL — normalize bare host:port to a full URL
	u, err := hubWebSocketURL(c.hubURL, "/ws/peer")
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close()

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	defer func() {
		c.ptyManager.CloseAll("control-disconnected")
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
	}()

	log.Info("connected to hub")

	// Step 1: Read challenge
	var challengeMsg Message
	if err := conn.ReadJSON(&challengeMsg); err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}
	if challengeMsg.Type != MsgChallenge {
		return fmt.Errorf("expected challenge, got %s", challengeMsg.Type)
	}

	var challenge ChallengePayload
	if err := json.Unmarshal(challengeMsg.Payload, &challenge); err != nil {
		return fmt.Errorf("parse challenge: %w", err)
	}

	challengeBytes, err := base64.StdEncoding.DecodeString(challenge.Challenge)
	if err != nil {
		return fmt.Errorf("decode challenge: %w", err)
	}

	// Step 2: Sign and respond
	sig, err := c.identity.Sign(challengeBytes)
	if err != nil {
		return fmt.Errorf("sign challenge: %w", err)
	}

	authMsg, _ := NewMessage(MsgAuth, AuthPayload{
		PublicKey: c.identity.PublicKey,
		Signature: base64.StdEncoding.EncodeToString(sig),
	})
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	// Step 3: Read auth result
	var resultMsg Message
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := conn.ReadJSON(&resultMsg); err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}
	conn.SetReadDeadline(time.Time{})

	if resultMsg.Type == MsgAuthFail {
		return fmt.Errorf("authentication failed")
	}
	if resultMsg.Type != MsgAuthOK {
		return fmt.Errorf("unexpected message: %s", resultMsg.Type)
	}

	log.Info("authenticated with hub")

	helloMessage, _ := NewMessage(MsgHello, HelloPayload{
		MinVersion: RuntimeProtocolMin, MaxVersion: RuntimeProtocolMax,
		Capabilities: RuntimeCapabilities, BuildVersion: common.VERSION,
		AgentInstanceID: c.instanceID,
	})
	if err := conn.WriteJSON(helloMessage); err != nil {
		return fmt.Errorf("send runtime hello: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var helloAckMessage Message
	if err := conn.ReadJSON(&helloAckMessage); err != nil {
		return fmt.Errorf("read runtime hello ack: %w", err)
	}
	if helloAckMessage.Type == MsgRuntimeError {
		var runtimeError RuntimeError
		_ = json.Unmarshal(helloAckMessage.Payload, &runtimeError)
		return fmt.Errorf("runtime negotiation failed: %w", runtimeError)
	}
	if helloAckMessage.Type != MsgHelloAck {
		return fmt.Errorf("expected runtime hello ack, got %s", helloAckMessage.Type)
	}
	var helloAck HelloAckPayload
	if err := json.Unmarshal(helloAckMessage.Payload, &helloAck); err != nil {
		return fmt.Errorf("parse runtime hello ack: %w", err)
	}
	if err := helloAck.Validate(); err != nil || helloAck.AgentInstanceID != c.instanceID {
		return fmt.Errorf("invalid runtime hello ack: %v", err)
	}
	c.mu.Lock()
	c.runtimeGeneration = helloAck.Generation
	c.runtimeCapabilities = make(map[string]struct{}, len(helloAck.Capabilities))
	for _, capability := range helloAck.Capabilities {
		c.runtimeCapabilities[capability] = struct{}{}
	}
	c.dispatcher = &RequestDispatcher{
		Generation: helloAck.Generation, HostID: c.peerMgr.LocalID(),
		Capabilities: c.runtimeCapabilities, Executor: tmuxRuntimeExecutor{client: c.tmuxClient},
		Cache: c.outcomeCache,
	}
	c.mu.Unlock()
	log.WithField("generation", helloAck.Generation).Info("runtime protocol negotiated")

	// Configure ping/pong for connection liveness detection
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Send initial state
	c.sendStateUpdate(conn)

	// Start periodic senders
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	go c.pingLoop(ctx2, conn)
	go c.periodicActivity(ctx2, conn)
	go c.periodicStats(ctx2, conn)
	go c.forwardStateEvents(ctx2, conn)
	go c.forwardToolEvents(ctx2, conn)

	// Read loop: process messages from hub
	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			return fmt.Errorf("read from hub: %w", err)
		}

		c.handleHubMessage(&msg, conn, log)
	}
}

func randomInstanceID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic("system random source unavailable: " + err.Error())
	}
	return hex.EncodeToString(value)
}

func (c *Client) handleHubMessage(msg *Message, conn *websocket.Conn, log *logrus.Entry) {
	switch msg.Type {
	case MsgRuntimeRequest:
		var request RuntimeRequest
		if err := json.Unmarshal(msg.Payload, &request); err != nil {
			log.WithError(err).Debug("invalid runtime request")
			return
		}
		go c.handleRuntimeRequest(request, conn)
	case MsgPeerState:
		var payload PeerStatePayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid peer-state")
			return
		}
		// Update peer manager with remote hosts
		for _, host := range payload.Hosts {
			if c.peerMgr.IsLocal(host.ID) {
				continue
			}
			c.peerMgr.UpdatePeerSessions(host.ID, host.Sessions)
			if host.Online {
				// Register host if not already known
				if !c.peerMgr.HasHost(host.ID) {
					c.peerMgr.RegisterPeer(host.ID, host.Name, "", nil)
				}
			}
		}

	case MsgPeerConnected:
		var payload PeerNotifyPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		c.peerMgr.RegisterPeer(payload.ID, payload.Name, "", nil)

	case MsgPeerDisconnected:
		var payload PeerNotifyPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}
		c.peerMgr.UnregisterPeer(payload.ID)

	case MsgPTYOpen:
		var payload PTYOpenPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			log.WithError(err).Debug("invalid pty-open")
			return
		}
		go c.ptyManager.Open(payload)

	case MsgRequestState:
		c.sendStateUpdate(conn)

	default:
		log.WithField("type", msg.Type).Debug("unknown message from hub")
	}
}

func (c *Client) handleRuntimeRequest(request RuntimeRequest, conn *websocket.Conn) {
	c.mu.Lock()
	dispatcher := c.dispatcher
	c.mu.Unlock()
	if dispatcher == nil {
		return
	}
	ack, outcome := dispatcher.Dispatch(context.Background(), request)
	if ack.Accepted {
		if message, err := NewMessage(MsgRuntimeAck, ack); err == nil {
			c.writeJSON(conn, message)
		}
	}
	if outcome.Error != nil {
		if message, err := NewMessage(MsgRuntimeError, outcome.Error); err == nil {
			c.writeJSON(conn, message)
		}
		return
	}
	if message, err := NewMessage(MsgRuntimeResult, outcome.Result); err == nil {
		c.writeJSON(conn, message)
	}
}

func (c *Client) sendStateUpdate(conn *websocket.Conn) {
	sessions := c.localMgr.GetSessions()
	msg, err := NewMessage(MsgStateUpdate, StateUpdatePayload{Sessions: sessions, Version: common.VERSION})
	if err != nil {
		return
	}
	c.writeJSON(conn, msg)
}

func (c *Client) forwardStateEvents(ctx context.Context, conn *websocket.Conn) {
	ch := c.localMgr.Subscribe()
	defer c.localMgr.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			msg, err := NewMessage(MsgStateEvent, StateEventPayload{
				EventType: evt.Type,
				Session:   evt.Session,
			})
			if err != nil {
				continue
			}
			c.writeJSON(conn, msg)

			// Also send full state update on change
			c.sendStateUpdate(conn)
		}
	}
}

func (c *Client) forwardToolEvents(ctx context.Context, conn *websocket.Conn) {
	ch := c.toolTracker.Subscribe()
	defer c.toolTracker.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			logrus.WithFields(logrus.Fields{
				"tool":    evt.Tool,
				"status":  evt.Status,
				"session": evt.Session,
			}).Debug("forwarding tool event to hub")
			msg, err := NewMessage(MsgToolEvent, ToolEventPayload{Event: evt})
			if err != nil {
				continue
			}
			c.writeJSON(conn, msg)
		}
	}
}

func (c *Client) pingLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) periodicActivity(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshots := c.actTracker.GetAll()
			// Stamp each snapshot with our host ID so the hub can key them correctly
			localID := c.peerMgr.LocalID()
			for _, s := range snapshots {
				if s.Host == "" {
					s.Host = localID
				}
			}
			msg, err := NewMessage(MsgActivityUpdate, ActivityUpdatePayload{Snapshots: snapshots})
			if err != nil {
				continue
			}
			c.writeJSON(conn, msg)
		}
	}
}

func (c *Client) collectStats() map[string]interface{} {
	s := stats.SystemStats()
	sessions := c.localMgr.GetSessions()
	s["processes"] = stats.ProcessCountsFromSessions(sessions)
	return s
}

func (c *Client) periodicStats(ctx context.Context, conn *websocket.Conn) {
	// Send immediately on connect
	if msg, err := NewMessage(MsgStats, StatsPayload{Stats: c.collectStats()}); err == nil {
		c.writeJSON(conn, msg)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if msg, err := NewMessage(MsgStats, StatsPayload{Stats: c.collectStats()}); err == nil {
				c.writeJSON(conn, msg)
			}
		}
	}
}

func (c *Client) writeJSON(conn *websocket.Conn, msg *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn.WriteJSON(msg)
}

// HubURL returns the hub URL for PTY connections
func (c *Client) HubURL() string {
	return c.hubURL
}

func (c *Client) RuntimeGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runtimeGeneration
}
