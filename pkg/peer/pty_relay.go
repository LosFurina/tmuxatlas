package peer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const ptyAttachDeadline = 15 * time.Second

// PTYRelay owns the pending and active data-channel index. Each stream itself
// is owned by PTYOwner and by exactly one control connection generation.
type PTYRelay struct {
	mu            sync.RWMutex
	pending       map[string]*PTYOwner
	active        map[string]*PTYOwner
	attachTimeout time.Duration
}

type PTYOwner struct {
	StreamID    string
	HostID      string
	Generation  uint64
	Target      SessionTarget
	AttachToken string

	relay      *PTYRelay
	connection *PeerConnection
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	ready      chan struct{}
	BrowserWS  *websocket.Conn

	mu     sync.RWMutex
	PeerWS *websocket.Conn
	wg     sync.WaitGroup
}

func NewPTYRelay() *PTYRelay {
	return &PTYRelay{
		pending:       make(map[string]*PTYOwner),
		active:        make(map[string]*PTYOwner),
		attachTimeout: ptyAttachDeadline,
	}
}

func GenerateStreamID() string {
	return randomPTYSecret()
}

func randomPTYSecret() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic("system random source unavailable: " + err.Error())
	}
	return hex.EncodeToString(value)
}

func (relay *PTYRelay) RegisterPending(connection *PeerConnection, target SessionTarget, browserWS *websocket.Conn) (*PTYOwner, error) {
	if connection == nil {
		return nil, ErrPeerOffline
	}
	ctx, cancel := context.WithCancel(connection.ctx)
	owner := &PTYOwner{
		StreamID: randomPTYSecret(), HostID: connection.HostID, Generation: connection.Generation,
		Target: target, AttachToken: randomPTYSecret(), relay: relay, connection: connection,
		ctx: ctx, cancel: cancel, ready: make(chan struct{}), BrowserWS: browserWS,
	}
	if err := connection.ownPTY(owner); err != nil {
		cancel()
		return nil, err
	}
	relay.mu.Lock()
	relay.pending[owner.StreamID] = owner
	relay.mu.Unlock()
	time.AfterFunc(relay.attachTimeout, func() {
		select {
		case <-owner.ready:
		default:
			owner.Teardown("attach-timeout")
		}
	})
	return owner, nil
}

func (relay *PTYRelay) CompletePending(streamID, hostID string, generation uint64, token, session string, peerWS *websocket.Conn) bool {
	relay.mu.Lock()
	owner := relay.pending[streamID]
	if owner == nil || owner.HostID != hostID || owner.Generation != generation ||
		owner.AttachToken != token || owner.Target.Session != session {
		relay.mu.Unlock()
		return false
	}
	select {
	case <-owner.ctx.Done():
		relay.mu.Unlock()
		return false
	default:
	}
	delete(relay.pending, streamID)
	relay.active[streamID] = owner
	owner.mu.Lock()
	owner.PeerWS = peerWS
	owner.mu.Unlock()
	close(owner.ready)
	relay.mu.Unlock()
	return true
}

func (owner *PTYOwner) Ready() <-chan struct{} {
	return owner.ready
}

func (owner *PTYOwner) Teardown(reason string) {
	owner.once.Do(func() {
		owner.cancel()
		owner.relay.mu.Lock()
		delete(owner.relay.pending, owner.StreamID)
		delete(owner.relay.active, owner.StreamID)
		owner.relay.mu.Unlock()
		owner.connection.releasePTY(owner.StreamID)
		if owner.BrowserWS != nil {
			_ = owner.BrowserWS.Close()
		}
		owner.mu.RLock()
		peerWS := owner.PeerWS
		owner.mu.RUnlock()
		if peerWS != nil {
			control, _ := EncodePTYControlFrame(PTYControlFrame{
				Version: PTYFrameVersion, Type: "close", Sequence: 1, Reason: reason,
			})
			_ = peerWS.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, string(control)),
				time.Now().Add(time.Second))
			_ = peerWS.Close()
		}
	})
}

func (owner *PTYOwner) Wait() {
	owner.wg.Wait()
}

func (relay *PTYRelay) HandlePeerPTY(w http.ResponseWriter, request *http.Request) {
	streamID := request.URL.Query().Get("stream")
	hostID := request.URL.Query().Get("host")
	token := request.URL.Query().Get("token")
	session := request.URL.Query().Get("session")
	generation, err := strconv.ParseUint(request.URL.Query().Get("generation"), 10, 64)
	if streamID == "" || hostID == "" || token == "" || session == "" || err != nil || generation == 0 {
		http.Error(w, "invalid PTY attachment identity", http.StatusBadRequest)
		return
	}
	conn, err := wsUpgrader.Upgrade(w, request, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(maxPTYFrameData + ptyDataHeaderSize)
	if !relay.CompletePending(streamID, hostID, generation, token, session, conn) {
		logrus.WithField("stream", streamID).Warn("rejected unmatched or late PTY attachment")
		_ = conn.Close()
		return
	}
}

// Bridge translates the browser's raw xterm messages into protocol-v1 frames.
func (owner *PTYOwner) Bridge() {
	owner.mu.RLock()
	peerWS := owner.PeerWS
	owner.mu.RUnlock()
	if peerWS == nil || owner.BrowserWS == nil {
		owner.Teardown("missing-endpoint")
		return
	}
	browserWS := owner.BrowserWS
	browserWS.SetReadLimit(maxPTYFrameData)
	peerWS.SetReadLimit(maxPTYFrameData + ptyDataHeaderSize)

	owner.wg.Add(2)
	done := make(chan struct{}, 2)
	go func() {
		defer owner.wg.Done()
		defer func() { done <- struct{}{} }()
		var outputSequence PTYSequence
		for {
			messageType, data, err := peerWS.ReadMessage()
			if err != nil {
				return
			}
			switch messageType {
			case websocket.BinaryMessage:
				frame, err := DecodePTYDataFrame(data)
				if err != nil || frame.Direction != PTYDataOutput {
					return
				}
				duplicate, err := outputSequence.Accept(frame.Sequence)
				if err != nil {
					return
				}
				if duplicate {
					continue
				}
				if err := browserWS.WriteMessage(websocket.BinaryMessage, frame.Payload); err != nil {
					return
				}
			case websocket.TextMessage:
				frame, err := DecodePTYControlFrame(data)
				if err != nil {
					return
				}
				if frame.Type == "close" || frame.Type == "error" {
					return
				}
			default:
				return
			}
		}
	}()
	go func() {
		defer owner.wg.Done()
		defer func() { done <- struct{}{} }()
		var dataSequence, controlSequence uint64
		for {
			messageType, data, err := browserWS.ReadMessage()
			if err != nil {
				return
			}
			switch messageType {
			case websocket.BinaryMessage:
				dataSequence++
				frame, err := EncodePTYDataFrame(PTYDataInput, dataSequence, data)
				if err != nil || peerWS.WriteMessage(websocket.BinaryMessage, frame) != nil {
					return
				}
			case websocket.TextMessage:
				var browserResize struct {
					Type string `json:"type"`
					Cols uint16 `json:"cols"`
					Rows uint16 `json:"rows"`
				}
				if err := jsonUnmarshalStrict(data, &browserResize); err != nil ||
					browserResize.Type != "resize" || browserResize.Cols == 0 || browserResize.Rows == 0 {
					return
				}
				controlSequence++
				frame, err := EncodePTYControlFrame(PTYControlFrame{
					Version: PTYFrameVersion, Type: "resize", Sequence: controlSequence,
					Cols: browserResize.Cols, Rows: browserResize.Rows,
				})
				if err != nil || peerWS.WriteMessage(websocket.TextMessage, frame) != nil {
					return
				}
			default:
				return
			}
		}
	}()
	<-done
	owner.Teardown("stream-ended")
	owner.Wait()
	logrus.WithField("stream", owner.StreamID).Debug("PTY relay bridge closed")
}

func jsonUnmarshalStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}
