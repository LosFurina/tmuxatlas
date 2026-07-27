package peer

import (
	"context"
	"fmt"
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
	ID            string // public key fingerprint
	Name          string
	Version       string
	PublicKey     string
	Sessions      []*tmux.Session
	Stats         map[string]interface{}
	Activity      []*activity.Snapshot
	ToolEvents    []*toolevents.Event
	Connected     bool
	LastSeen      time.Time
	Conn          *PeerConnection // nil for local host
	Generation    uint64
	Capabilities  []string
	AgentInstance string
}

// Manager aggregates state from local tmux and remote peers
type Manager struct {
	mu    sync.RWMutex
	hosts map[string]*HostState // keyed by peer fingerprint

	localID     string // this node's fingerprint
	localName   string
	hasLocal    bool
	identity    *identity.Identity
	peerStore   *identity.PeerStore
	localMgr    *state.Manager
	generations map[string]uint64

	// Subscribers for state changes (browser WebSocket hub subscribes here)
	subMu       sync.RWMutex
	subscribers []chan state.StateEvent
}

// NewManager creates a new peer manager
func NewManager(id *identity.Identity, peerStore *identity.PeerStore, localMgr *state.Manager) *Manager {
	return newManager(id, peerStore, localMgr, true)
}

// NewHubManager creates a remote-only manager. The Hub identity authenticates
// peer traffic but is deliberately not registered as a tmux-capable host.
func NewHubManager(id *identity.Identity, peerStore *identity.PeerStore) *Manager {
	return newManager(id, peerStore, nil, false)
}

func newManager(id *identity.Identity, peerStore *identity.PeerStore, localMgr *state.Manager, hasLocal bool) *Manager {
	m := &Manager{
		hosts:       make(map[string]*HostState),
		localID:     id.Fingerprint(),
		localName:   id.Name,
		hasLocal:    hasLocal,
		identity:    id,
		peerStore:   peerStore,
		localMgr:    localMgr,
		generations: make(map[string]uint64),
	}

	if hasLocal {
		m.hosts[m.localID] = &HostState{
			ID:        m.localID,
			Name:      id.Name,
			Version:   common.VERSION,
			PublicKey: id.PublicKey,
			Connected: true,
			LastSeen:  time.Now(),
		}
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
	m.RunContext(context.Background())
}

// RunContext forwards optional local state and prunes remote peers until the
// runtime is cancelled.
func (m *Manager) RunContext(ctx context.Context) {
	var localCh chan state.StateEvent
	if m.localMgr != nil {
		localCh = m.localMgr.Subscribe()
		defer m.localMgr.Unsubscribe(localCh)
	}
	pruneTimer := time.NewTicker(30 * time.Second)
	defer pruneTimer.Stop()

	var statsTimer *time.Ticker
	var statsC <-chan time.Time
	if m.localMgr != nil {
		statsTimer = time.NewTicker(30 * time.Second)
		statsC = statsTimer.C
		defer statsTimer.Stop()
		m.updateLocalStats()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-localCh:
			if !ok {
				localCh = nil
				continue
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

		case <-statsC:
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
	if m.localMgr == nil {
		return nil
	}
	sessions := m.localMgr.GetSessions()
	for _, session := range sessions {
		session.Host = m.localID
		session.HostName = m.localName
		session.HostOnline = true
	}
	return sessions
}

// GetHosts returns info about all known hosts
func (m *Manager) GetHosts() []HostInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hosts := make([]HostInfo, 0, len(m.hosts))
	for _, h := range m.hosts {
		hosts = append(hosts, HostInfo{
			ID: h.ID, Name: h.Name, Version: h.Version,
			RuntimeProtocol: RuntimeProtocolMax, Generation: h.Generation,
			Capabilities: h.Capabilities, AgentInstance: h.AgentInstance,
			Local: m.hasLocal && h.ID == m.localID, Online: h.Connected, Sessions: h.Sessions,
			Activity: h.Activity, Stats: h.Stats, LastSeen: h.LastSeen,
		})
	}
	return hosts
}

// StateHostSnapshots returns a defensive producer snapshot for the canonical
// browser-state coordinator. It intentionally excludes live connections and
// other Peer lifecycle objects.
func (m *Manager) StateHostSnapshots() []state.HostSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hosts := make([]state.HostSnapshot, 0, len(m.hosts))
	for _, host := range m.hosts {
		sessions := make([]*tmux.Session, 0, len(host.Sessions))
		for _, session := range host.Sessions {
			sessions = append(sessions, cloneTmuxSession(session))
		}
		hosts = append(hosts, state.HostSnapshot{
			ID: host.ID, DisplayName: host.Name, Online: host.Connected,
			Local: m.hasLocal && host.ID == m.localID, Version: host.Version, Sessions: sessions,
		})
	}
	return hosts
}

func cloneTmuxSession(source *tmux.Session) *tmux.Session {
	if source == nil {
		return nil
	}
	target := *source
	target.Windows = make([]*tmux.Window, 0, len(source.Windows))
	for _, sourceWindow := range source.Windows {
		if sourceWindow == nil {
			target.Windows = append(target.Windows, nil)
			continue
		}
		targetWindow := *sourceWindow
		targetWindow.Panes = make([]*tmux.Pane, 0, len(sourceWindow.Panes))
		for _, sourcePane := range sourceWindow.Panes {
			if sourcePane == nil {
				targetWindow.Panes = append(targetWindow.Panes, nil)
				continue
			}
			targetPane := *sourcePane
			targetWindow.Panes = append(targetWindow.Panes, &targetPane)
		}
		target.Windows = append(target.Windows, &targetWindow)
	}
	return &target
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

func (m *Manager) ReserveGeneration(id string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generations[id]++
	return m.generations[id]
}

// ActivatePeer atomically installs a negotiated generation. It returns false
// if a newer generation won the race. Cancellation of the replaced connection
// happens after the registry lock is released.
func (m *Manager) ActivatePeer(id, name, publicKey string, conn *PeerConnection) bool {
	m.mu.Lock()
	return m.activatePeerLocked(id, name, publicKey, conn, false)
}

// ActivateAuthorizedPeer performs the final authorization check under the same
// manager lock used by runtime revocation.
func (m *Manager) ActivateAuthorizedPeer(id, name, publicKey string, conn *PeerConnection) bool {
	m.mu.Lock()
	return m.activatePeerLocked(id, name, publicKey, conn, true)
}

func (m *Manager) activatePeerLocked(id, name, publicKey string, conn *PeerConnection, requireAuthorization bool) bool {
	if requireAuthorization && (m.peerStore == nil || m.peerStore.GetByPublicKey(publicKey) == nil) {
		m.mu.Unlock()
		return false
	}
	current := m.hosts[id]
	if current != nil && current.Conn != nil && current.Generation >= conn.Generation {
		m.mu.Unlock()
		return false
	}
	var previous *PeerConnection
	if current != nil {
		previous = current.Conn
	}
	capabilities := make([]string, 0, len(conn.Capabilities))
	for capability := range conn.Capabilities {
		capabilities = append(capabilities, capability)
	}
	m.hosts[id] = &HostState{
		ID: id, Name: name, PublicKey: publicKey, Connected: true,
		LastSeen: time.Now(), Conn: conn, Generation: conn.Generation,
		Capabilities: capabilities, AgentInstance: conn.AgentInstance,
	}
	m.mu.Unlock()

	if previous != nil {
		if previous.AgentInstance != "" && previous.AgentInstance != conn.AgentInstance {
			previous.CloseWith(ErrorExecutionUnknown)
		} else {
			previous.Close()
		}
	}
	conn.Start()
	m.broadcast(state.StateEvent{Type: "peer-connected", Host: id, HostName: name})
	return true
}

// RevokePeer atomically commits authorization removal before changing runtime
// state. Network and goroutine cancellation occur after the registry lock.
func (m *Manager) RevokePeer(name string) (identity.Peer, error) {
	m.mu.Lock()
	if m.peerStore == nil {
		m.mu.Unlock()
		return identity.Peer{}, fmt.Errorf("peer store unavailable")
	}
	authorized := m.peerStore.Get(name)
	if authorized == nil {
		m.mu.Unlock()
		return identity.Peer{}, fmt.Errorf("peer %q not found", name)
	}
	if err := m.peerStore.Remove(name); err != nil {
		m.mu.Unlock()
		return identity.Peer{}, err
	}
	fingerprint := authorized.Fingerprint()
	host := m.hosts[fingerprint]
	delete(m.hosts, fingerprint)
	var connection *PeerConnection
	if host != nil {
		connection = host.Conn
	}
	m.mu.Unlock()

	if connection != nil {
		connection.CloseWith(ErrorPeerRevoked)
	}
	m.broadcast(state.StateEvent{
		Type: "peer-disconnected", Host: fingerprint, HostName: authorized.Name,
	})
	return *authorized, nil
}

func (m *Manager) UnregisterPeerGeneration(id string, generation uint64) bool {
	m.mu.Lock()
	host, ok := m.hosts[id]
	if !ok || host.Generation != generation {
		m.mu.Unlock()
		return false
	}
	host.Connected = false
	host.Conn = nil
	host.LastSeen = time.Now()
	name := host.Name
	m.mu.Unlock()
	m.broadcast(state.StateEvent{Type: "peer-disconnected", Host: id, HostName: name})
	return true
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
	h, ok := m.hosts[id]
	if ok {
		h.Version = version
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()
	if ok {
		m.broadcast(state.StateEvent{Type: "host-health-changed", Host: id, HostName: h.Name})
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
	h, ok := m.hosts[id]
	if ok {
		h.Stats = stats
		h.LastSeen = time.Now()
	}
	m.mu.Unlock()
	if ok {
		m.broadcast(state.StateEvent{Type: "host-health-changed", Host: id, HostName: h.Name})
	}
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

func (m *Manager) IsCurrent(connection *PeerConnection) bool {
	if connection == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	host := m.hosts[connection.HostID]
	return host != nil && host.Conn == connection && host.Generation == connection.Generation
}

// GetAllActivity returns activity snapshots from all remote peers (not local)
func (m *Manager) GetAllActivity() []*activity.Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*activity.Snapshot
	for id, h := range m.hosts {
		if m.hasLocal && id == m.localID {
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
	return m.hasLocal && hostID == m.localID
}

func (m *Manager) HasSession(hostID, session string) bool {
	if m.hasLocal && hostID == m.localID && m.localMgr != nil {
		for _, candidate := range m.localMgr.GetSessions() {
			if candidate.Name == session {
				return true
			}
		}
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	host := m.hosts[hostID]
	if host == nil {
		return false
	}
	for _, candidate := range host.Sessions {
		if candidate.Name == session {
			return true
		}
	}
	return false
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
