package peer

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/stats"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

const (
	// OfflineTimeout is how long to keep an offline peer's sessions visible
	OfflineTimeout = 5 * time.Minute
)

// HostState holds all known state for a single peer
type HostState struct {
	ID         string // public key fingerprint
	Name       string
	Version    string
	PublicKey  string
	Sessions   []*tmux.Session
	Stats      map[string]interface{}
	Activity   []*activity.Snapshot
	ToolEvents []*toolevents.Event
	Connected  bool
	LastSeen   time.Time
	Conn       *PeerConnection // nil for local host
}

// PeerConnection wraps a control WebSocket to a peer
type PeerConnection struct {
	HostID string
	Send   chan *Message // messages queued for sending to this peer
}

// Manager aggregates state from local tmux and remote peers
type Manager struct {
	mu    sync.RWMutex
	hosts map[string]*HostState // keyed by peer fingerprint

	localID   string // this node's fingerprint
	localName string
	identity  *identity.Identity
	peerStore *identity.PeerStore
	localMgr  *state.Manager

	// Subscribers for state changes (browser WebSocket hub subscribes here)
	subMu       sync.RWMutex
	subscribers []chan state.StateEvent
}

// NewManager creates a new peer manager
func NewManager(id *identity.Identity, peerStore *identity.PeerStore, localMgr *state.Manager) *Manager {
	m := &Manager{
		hosts:     make(map[string]*HostState),
		localID:   id.Fingerprint(),
		localName: id.Name,
		identity:  id,
		peerStore: peerStore,
		localMgr:  localMgr,
	}

	// Register local host
	m.hosts[m.localID] = &HostState{
		ID:        m.localID,
		Name:      id.Name,
		Version:   common.VERSION,
		PublicKey: id.PublicKey,
		Connected: true,
		LastSeen:  time.Now(),
	}

	return m
}

// updateLocalStats collects system stats and process counts for the local host
func (m *Manager) updateLocalStats() {
	s := stats.SystemStats()
	sessions := m.localMgr.GetSessions()
	s["processes"] = stats.ProcessCountsFromSessions(sessions)
	m.UpdatePeerStats(m.localID, s)
}

// Run starts forwarding local state events to peer manager subscribers
// and pruning offline peers
func (m *Manager) Run() {
	// Forward local state events
	localCh := m.localMgr.Subscribe()
	defer m.localMgr.Unsubscribe(localCh)

	pruneTimer := time.NewTicker(30 * time.Second)
	defer pruneTimer.Stop()

	statsTimer := time.NewTicker(30 * time.Second)
	defer statsTimer.Stop()

	// Collect initial stats
	m.updateLocalStats()

	for {
		select {
		case evt, ok := <-localCh:
			if !ok {
				return
			}
			// Stamp with local host info
			evt.Host = m.localID
			evt.HostName = m.localName

			// Update local sessions cache
			m.mu.Lock()
			if h, ok := m.hosts[m.localID]; ok {
				h.Sessions = m.localMgr.GetSessions()
				h.LastSeen = time.Now()
			}
			m.mu.Unlock()

			m.broadcast(evt)

		case <-statsTimer.C:
			m.updateLocalStats()

		case <-pruneTimer.C:
			m.pruneOffline()
		}
	}
}

// Subscribe returns a channel that receives state events from all hosts
func (m *Manager) Subscribe() chan state.StateEvent {
	ch := make(chan state.StateEvent, 64)
	m.subMu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel
func (m *Manager) Unsubscribe(ch chan state.StateEvent) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for i, sub := range m.subscribers {
		if sub == ch {
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// broadcast sends an event to all subscribers
func (m *Manager) broadcast(evt state.StateEvent) {
	m.subMu.RLock()
	defer m.subMu.RUnlock()
	for _, ch := range m.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}

// GetAllSessions returns sessions from all hosts, with host fields stamped
func (m *Manager) GetAllSessions() []*tmux.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*tmux.Session
	for _, h := range m.hosts {
		for _, s := range h.Sessions {
			s.Host = h.ID
			s.HostName = h.Name
			s.HostOnline = h.Connected
			all = append(all, s)
		}
	}
	return all
}

// GetLocalSessions returns only this node's sessions
func (m *Manager) GetLocalSessions() []*tmux.Session {
	return m.localMgr.GetSessions()
}

// GetHosts returns info about all known hosts
func (m *Manager) GetHosts() []HostInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hosts := make([]HostInfo, 0, len(m.hosts))
	for _, h := range m.hosts {
		hosts = append(hosts, HostInfo{
			ID:       h.ID,
			Name:     h.Name,
			Version:  h.Version,
			Local:    h.ID == m.localID,
			Online:   h.Connected,
			Sessions: h.Sessions,
			Activity: h.Activity,
			Stats:    h.Stats,
			LastSeen: h.LastSeen,
		})
	}
	return hosts
}

// LocalID returns this node's fingerprint
func (m *Manager) LocalID() string {
	return m.localID
}

// LocalName returns this node's display name
func (m *Manager) LocalName() string {
	return m.localName
}

// RegisterPeer registers a newly connected peer
func (m *Manager) RegisterPeer(id, name, publicKey string, conn *PeerConnection) {
	m.mu.Lock()
	m.hosts[id] = &HostState{
		ID:        id,
		Name:      name,
		PublicKey: publicKey,
		Connected: true,
		LastSeen:  time.Now(),
		Conn:      conn,
	}
	m.mu.Unlock()

	m.broadcast(state.StateEvent{
		Type:     "peer-connected",
		Host:     id,
		HostName: name,
	})

	logrus.WithFields(logrus.Fields{
		"peer": name,
		"id":   id,
	}).Info("peer connected")
}

// UnregisterPeer marks a peer as disconnected
func (m *Manager) UnregisterPeer(id string) {
	m.mu.Lock()
	h, ok := m.hosts[id]
	if ok {
		h.Connected = false
		h.Conn = nil
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()

	if ok {
		m.broadcast(state.StateEvent{
			Type:     "peer-disconnected",
			Host:     id,
			HostName: h.Name,
		})

		logrus.WithFields(logrus.Fields{
			"peer": h.Name,
			"id":   id,
		}).Info("peer disconnected")
	}
}

// UpdatePeerSessions updates a peer's session list
func (m *Manager) UpdatePeerSessions(id string, sessions []*tmux.Session) {
	m.mu.Lock()
	h, ok := m.hosts[id]
	if ok {
		h.Sessions = sessions
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()

	if ok {
		m.broadcast(state.StateEvent{
			Type:     "sessions-changed",
			Host:     id,
			HostName: h.Name,
		})
	}
}

// UpdatePeerVersion updates a peer's reported version
func (m *Manager) UpdatePeerVersion(id, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hosts[id]; ok {
		h.Version = version
	}
}

// UpdatePeerActivity updates a peer's activity snapshots
func (m *Manager) UpdatePeerActivity(id string, snapshots []*activity.Snapshot) {
	m.mu.Lock()
	if h, ok := m.hosts[id]; ok {
		h.Activity = snapshots
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()
}

// UpdatePeerStats updates a peer's system stats
func (m *Manager) UpdatePeerStats(id string, stats map[string]interface{}) {
	m.mu.Lock()
	if h, ok := m.hosts[id]; ok {
		h.Stats = stats
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()
}

// GetPeerConnection returns the connection for a specific peer
func (m *Manager) GetPeerConnection(id string) *PeerConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if h, ok := m.hosts[id]; ok {
		return h.Conn
	}
	return nil
}

// GetAllActivity returns activity snapshots from all remote peers (not local)
func (m *Manager) GetAllActivity() []*activity.Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*activity.Snapshot
	for id, h := range m.hosts {
		if id == m.localID {
			continue
		}
		all = append(all, h.Activity...)
	}
	return all
}

// GetHostName returns the display name for a host ID
func (m *Manager) GetHostName(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if h, ok := m.hosts[id]; ok {
		return h.Name
	}
	return ""
}

// HasHost returns true if a host with the given ID is known
func (m *Manager) HasHost(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.hosts[id]
	return ok
}

// IsLocal returns true if the given host ID is this node
func (m *Manager) IsLocal(hostID string) bool {
	return hostID == "" || hostID == m.localID
}

// pruneOffline removes peers that have been offline for too long
func (m *Manager) pruneOffline() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, h := range m.hosts {
		if id == m.localID {
			continue
		}
		if !h.Connected && now.Sub(h.LastSeen) > OfflineTimeout {
			delete(m.hosts, id)
			logrus.WithFields(logrus.Fields{
				"peer": h.Name,
				"id":   id,
			}).Info("pruned offline peer")
		}
	}
}
