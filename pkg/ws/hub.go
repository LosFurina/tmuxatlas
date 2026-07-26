package ws

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     CheckSameOrigin,
	ReadBufferSize:  1024,
	WriteBufferSize: 1024 * 16,
}

// CheckSameOrigin validates that the Origin header matches the Host header,
// preventing cross-site WebSocket hijacking from malicious web pages.
func CheckSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients (curl, CLI) don't send Origin
	}
	// Parse the origin to extract the host
	// Origin format: scheme://host[:port]
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// client wraps a WebSocket connection with a per-connection write mutex
type client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// ActivitySource provides activity snapshots for broadcasting.
// This avoids importing the peer package directly.
type ActivitySource interface {
	GetAllActivity() []*activity.Snapshot
}

// Hub manages WebSocket connections for state events and tool events
type Hub struct {
	mu              sync.RWMutex
	clients         map[*client]bool
	stateMgr        *state.Manager
	tracker         *toolevents.Tracker
	activityTracker *activity.Tracker
	peerActivity    ActivitySource // optional, for multi-host
	localHostID     string         // optional, set in multi-host mode
	localOnly       bool
}

// NewHub creates a new WebSocket hub
func NewHub(stateMgr *state.Manager, tracker *toolevents.Tracker) *Hub {
	return &Hub{
		clients:  make(map[*client]bool),
		stateMgr: stateMgr,
		tracker:  tracker,
	}
}

// SetActivityTracker configures the hub to broadcast activity snapshots
func (h *Hub) SetActivityTracker(at *activity.Tracker, peerActivity ActivitySource, localHostID string, localOnly bool) {
	h.activityTracker = at
	h.peerActivity = peerActivity
	h.localHostID = localHostID
	h.localOnly = localOnly
}

// Run starts broadcasting state events and tool events to connected clients
func (h *Hub) Run() {
	stateCh := h.stateMgr.Subscribe()
	defer h.stateMgr.Unsubscribe(stateCh)

	toolCh := h.tracker.Subscribe()
	defer h.tracker.Unsubscribe(toolCh)

	// Activity ticker — broadcast snapshots every 5 seconds
	var activityTicker *time.Ticker
	var activityCh <-chan time.Time
	if h.activityTracker != nil {
		activityTicker = time.NewTicker(5 * time.Second)
		activityCh = activityTicker.C
		defer activityTicker.Stop()
	}

	for {
		select {
		case evt, ok := <-stateCh:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				logrus.WithError(err).Warn("failed to marshal state event")
				continue
			}
			h.broadcastMessage(data)

		case evt, ok := <-toolCh:
			if !ok {
				return
			}
			// Wrap tool event with a type prefix so frontend can distinguish
			wrapped := map[string]interface{}{
				"type":          "tool-event",
				"tool":          evt.Tool,
				"status":        evt.Status,
				"host":          evt.Host,
				"host_name":     evt.HostName,
				"session":       evt.Session,
				"window":        evt.Window,
				"pane":          evt.Pane,
				"message":       evt.Message,
				"timestamp":     evt.Timestamp,
				"auto_detected": evt.AutoDetected,
			}
			data, err := json.Marshal(wrapped)
			if err != nil {
				logrus.WithError(err).Warn("failed to marshal tool event")
				continue
			}
			logrus.WithFields(logrus.Fields{
				"tool": evt.Tool, "status": evt.Status, "session": evt.Session,
				"pane": evt.Pane, "auto_detected": evt.AutoDetected,
			}).Trace("hub: broadcasting tool event to WebSocket clients")
			h.broadcastMessage(data)

		case <-activityCh:
			h.broadcastActivity()
		}
	}
}

// broadcastActivity sends activity snapshots to all connected clients
func (h *Hub) broadcastActivity() {
	h.mu.RLock()
	clientCount := len(h.clients)
	h.mu.RUnlock()
	if clientCount == 0 {
		return
	}

	snapshots := h.activityTracker.GetAll()

	// Stamp local host ID in multi-host mode
	if h.localHostID != "" {
		for _, s := range snapshots {
			if s.Host == "" {
				s.Host = h.localHostID
			}
		}
		// Merge peer activity if not local-only
		if !h.localOnly && h.peerActivity != nil {
			peerSnaps := h.peerActivity.GetAllActivity()
			snapshots = append(snapshots, peerSnaps...)
		}
	}

	wrapped := map[string]interface{}{
		"type":      "activity",
		"snapshots": snapshots,
	}
	data, err := json.Marshal(wrapped)
	if err != nil {
		logrus.WithError(err).Warn("failed to marshal activity")
		return
	}
	h.broadcastMessage(data)
}

// broadcastMessage sends a message to all connected clients
func (h *Hub) broadcastMessage(data []byte) {
	h.mu.RLock()
	// Snapshot clients under read lock
	snapshot := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		snapshot = append(snapshot, c)
	}
	h.mu.RUnlock()

	var failed []*client
	for _, c := range snapshot {
		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, data)
		c.mu.Unlock()
		if err != nil {
			logrus.WithError(err).Debug("failed to write to ws client")
			failed = append(failed, c)
		}
	}

	if len(failed) > 0 {
		h.mu.Lock()
		for _, c := range failed {
			c.conn.Close()
			delete(h.clients, c)
		}
		h.mu.Unlock()
	}
}

// HandleEvents handles WebSocket connections for state event streaming
func (h *Hub) HandleEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.WithError(err).Warn("ws upgrade failed")
		return
	}
	conn.SetReadLimit(64 << 10)
	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	})

	c := &client{conn: conn}

	// Send welcome message with server version
	welcome, _ := json.Marshal(map[string]string{
		"type":    "welcome",
		"version": common.VERSION,
		"commit":  common.COMMIT,
	})
	c.mu.Lock()
	_ = c.conn.WriteMessage(websocket.TextMessage, welcome)
	c.mu.Unlock()

	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	logrus.Debug("state ws client connected")
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					conn.Close()
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	// Keep connection alive by reading (and discarding) client messages
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	conn.Close()
	logrus.Debug("state ws client disconnected")
}
