package peer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

type PTYManager struct {
	mu       sync.RWMutex
	sessions map[string]*ActivePTY
	tmuxPath string
	activity *activity.Tracker
	client   *Client
	openPTY  func(string, string, uint16, uint16) (ptyDevice, error)
}

type ptyDevice interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(uint16, uint16) error
	Close()
}

type ActivePTY struct {
	StreamID string
	Target   SessionTarget
	PTY      ptyDevice
	HubWS    *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	wg     sync.WaitGroup
}

func NewPTYManager(tmuxPath string, actTracker *activity.Tracker, client *Client) *PTYManager {
	manager := &PTYManager{
		sessions: make(map[string]*ActivePTY),
		tmuxPath: tmuxPath,
		activity: actTracker,
		client:   client,
	}
	manager.openPTY = func(path, session string, cols, rows uint16) (ptyDevice, error) {
		return tmux.NewPTYSession(path, session, cols, rows)
	}
	return manager
}

func (pm *PTYManager) Open(request PTYOpenPayload) {
	log := logrus.WithFields(logrus.Fields{"stream": request.StreamID, "session": request.Target.Session})
	if request.StreamID == "" || request.AttachToken == "" || request.Generation == 0 ||
		request.Cols == 0 || request.Rows == 0 || request.Target.Validate() != nil ||
		request.Target.HostID != pm.client.peerMgr.LocalID() ||
		request.Generation != pm.client.RuntimeGeneration() {
		log.Warn("rejected invalid or stale PTY open")
		return
	}
	ptySession, err := pm.openPTY(pm.tmuxPath, request.Target.Session, request.Cols, request.Rows)
	if err != nil {
		log.WithError(err).Error("failed to spawn PTY")
		return
	}
	hubWS, err := pm.connectPTYWebSocket(request)
	if err != nil {
		log.WithError(err).Error("failed to attach PTY data channel")
		ptySession.Close()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	active := &ActivePTY{
		StreamID: request.StreamID, Target: request.Target, PTY: ptySession,
		HubWS: hubWS, ctx: ctx, cancel: cancel,
	}
	pm.mu.Lock()
	if previous := pm.sessions[request.StreamID]; previous != nil {
		pm.mu.Unlock()
		active.Teardown("duplicate-stream")
		return
	}
	active.wg.Add(2)
	pm.sessions[request.StreamID] = active
	pm.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() {
		defer active.wg.Done()
		defer func() { done <- struct{}{} }()
		var outputSequence uint64
		buffer := make([]byte, 32*1024)
		for {
			n, err := ptySession.Read(buffer)
			if err != nil {
				return
			}
			if pm.activity != nil {
				pm.activity.Record(request.Target.Session, n)
			}
			outputSequence++
			frame, err := EncodePTYDataFrame(PTYDataOutput, outputSequence, buffer[:n])
			if err != nil || hubWS.WriteMessage(websocket.BinaryMessage, frame) != nil {
				return
			}
		}
	}()
	go func() {
		defer active.wg.Done()
		defer func() { done <- struct{}{} }()
		var inputSequence, controlSequence PTYSequence
		for {
			messageType, data, err := hubWS.ReadMessage()
			if err != nil {
				return
			}
			switch messageType {
			case websocket.BinaryMessage:
				frame, err := DecodePTYDataFrame(data)
				if err != nil || frame.Direction != PTYDataInput {
					return
				}
				duplicate, err := inputSequence.Accept(frame.Sequence)
				if err != nil {
					return
				}
				if duplicate {
					continue
				}
				logPTYInputDiagnostic(request.Target.Session, frame.Payload)
				if _, err := ptySession.Write(frame.Payload); err != nil {
					return
				}
			case websocket.TextMessage:
				frame, err := DecodePTYControlFrame(data)
				if err != nil {
					return
				}
				duplicate, err := controlSequence.Accept(frame.Sequence)
				if err != nil {
					return
				}
				if duplicate {
					continue
				}
				switch frame.Type {
				case "resize":
					if err := ptySession.Resize(frame.Cols, frame.Rows); err != nil {
						return
					}
				case "close", "error":
					return
				}
			default:
				return
			}
		}
	}()

	<-done
	active.Teardown("relay-ended")
	active.wg.Wait()
	pm.mu.Lock()
	if pm.sessions[request.StreamID] == active {
		delete(pm.sessions, request.StreamID)
	}
	pm.mu.Unlock()
	log.Info("PTY relay stopped")
}

func logPTYInputDiagnostic(session string, payload []byte) {
	if os.Getenv("TMUXATLAS_DEBUG_PTY_INPUT") != "1" {
		return
	}
	if !bytes.Contains(payload, []byte("\x1b[<64;")) && !bytes.Contains(payload, []byte("\x1b[<65;")) {
		return
	}
	logrus.WithFields(logrus.Fields{
		"session": session,
		"bytes":   len(payload),
	}).Info("PTY input contains SGR mouse wheel sequence")
}

func (active *ActivePTY) Teardown(_ string) {
	active.once.Do(func() {
		active.cancel()
		_ = active.HubWS.Close()
		active.PTY.Close()
	})
}

func (pm *PTYManager) Close(streamID string) {
	pm.mu.RLock()
	active := pm.sessions[streamID]
	pm.mu.RUnlock()
	if active != nil {
		active.Teardown("control-close")
	}
}

func (pm *PTYManager) CloseAll(reason string) {
	pm.mu.RLock()
	active := make([]*ActivePTY, 0, len(pm.sessions))
	for _, session := range pm.sessions {
		active = append(active, session)
	}
	pm.mu.RUnlock()
	for _, session := range active {
		session.Teardown(reason)
	}
	for _, session := range active {
		session.wg.Wait()
	}
}

func (pm *PTYManager) Resize(streamID string, cols, rows uint16) {
	pm.mu.RLock()
	active := pm.sessions[streamID]
	pm.mu.RUnlock()
	if active != nil && cols > 0 && rows > 0 {
		_ = active.PTY.Resize(cols, rows)
	}
}

func (pm *PTYManager) connectPTYWebSocket(request PTYOpenPayload) (*websocket.Conn, error) {
	u, err := hubWebSocketURL(pm.client.HubURL(), "/ws/peer-pty")
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("stream", request.StreamID)
	query.Set("host", request.Target.HostID)
	query.Set("session", request.Target.Session)
	query.Set("generation", strconv.FormatUint(request.Generation, 10))
	query.Set("token", request.AttachToken)
	u.RawQuery = query.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("connect PTY data channel: %w", err)
	}
	conn.SetReadLimit(maxPTYFrameData + ptyDataHeaderSize)
	return conn, nil
}
