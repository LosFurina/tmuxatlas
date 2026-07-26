package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrPeerConflict = errors.New("peer identity conflict")

// Peer represents a known paired peer
type Peer struct {
	Name      string    `json:"name"`
	PublicKey string    `json:"public_key"`
	PairedAt  time.Time `json:"paired_at"`
}

// Fingerprint returns a short identifier derived from the peer's public key
func (p *Peer) Fingerprint() string {
	id := &Identity{PublicKey: p.PublicKey}
	return id.Fingerprint()
}

// PeerStore manages the list of known peers
type PeerStore struct {
	mu    sync.RWMutex
	path  string
	store peerStoreData
}

type peerStoreData struct {
	Peers []Peer `json:"peers"`
}

type legacyPeer struct {
	Name       string    `json:"name"`
	PublicKey  string    `json:"public_key"`
	PairedAt   time.Time `json:"paired_at"`
	TLSCertPEM string    `json:"tls_cert_pem,omitempty"`
	CACertPEM  string    `json:"ca_cert_pem,omitempty"`
}

type legacyPeerStoreData struct {
	Peers []legacyPeer `json:"peers"`
}

// NewPeerStore loads or creates the peer store
func NewPeerStore() (*PeerStore, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "peers.json")
	return loadPeerStore(path)
}

func loadPeerStore(path string) (*PeerStore, error) {
	ps := &PeerStore{path: path}

	data, err := os.ReadFile(path)
	if err == nil {
		var stored legacyPeerStoreData
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("parse peers: %w", err)
		}
		needsMigration := false
		for _, p := range stored.Peers {
			ps.store.Peers = append(ps.store.Peers, Peer{
				Name: p.Name, PublicKey: p.PublicKey, PairedAt: p.PairedAt,
			})
			needsMigration = needsMigration || p.TLSCertPEM != "" || p.CACertPEM != ""
		}
		if err := validatePeerRecords(ps.store.Peers); err != nil {
			return nil, fmt.Errorf("validate peers %s: %w", path, err)
		}
		if needsMigration {
			if err := backupPeerStore(path, data); err != nil {
				return nil, err
			}
			if err := ps.save(); err != nil {
				return nil, fmt.Errorf("migrate peers: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read peers: %w", err)
	}

	return ps, nil
}

func validatePeer(peer Peer) error {
	if err := validateStoredName(peer.Name); err != nil {
		return fmt.Errorf("invalid name: %w", err)
	}
	if _, err := ParsePublicKey(peer.PublicKey); err != nil {
		return err
	}
	return nil
}

func validatePeerRecords(peers []Peer) error {
	names := make(map[string]int, len(peers))
	keys := make(map[string]int, len(peers))
	for i, peer := range peers {
		if err := validatePeer(peer); err != nil {
			return fmt.Errorf("peers[%d] (%q): %w", i, peer.Name, err)
		}
		if previous, exists := names[peer.Name]; exists {
			return fmt.Errorf("peers[%d] (%q): duplicate name also used by peers[%d]", i, peer.Name, previous)
		}
		if previous, exists := keys[peer.PublicKey]; exists {
			return fmt.Errorf("peers[%d] (%q): duplicate public key also used by peers[%d]", i, peer.Name, previous)
		}
		names[peer.Name] = i
		keys[peer.PublicKey] = i
	}
	return nil
}

func backupPeerStore(path string, data []byte) error {
	backupPath := path + ".pre-system-trust.bak"
	f, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("backup legacy peers: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("backup legacy peers: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup legacy peers: %w", err)
	}
	return nil
}

// Add adds a peer to the store and persists to disk
func (ps *PeerStore) Add(peer Peer) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	normalizedName, err := NormalizeName(peer.Name)
	if err != nil {
		return fmt.Errorf("invalid peer: %w", err)
	}
	peer.Name = normalizedName
	if err := validatePeer(peer); err != nil {
		return fmt.Errorf("invalid peer: %w", err)
	}
	for _, existing := range ps.store.Peers {
		if existing.PublicKey == peer.PublicKey {
			return fmt.Errorf("%w: public key is already paired as %q", ErrPeerConflict, existing.Name)
		}
		if existing.Name == peer.Name {
			return fmt.Errorf("%w: name %q is already paired", ErrPeerConflict, peer.Name)
		}
	}

	updated := append(append([]Peer(nil), ps.store.Peers...), peer)
	if err := ps.saveData(peerStoreData{Peers: updated}); err != nil {
		return err
	}
	ps.store.Peers = updated
	return nil
}

// Remove removes a peer by name and persists to disk
func (ps *PeerStore) Remove(name string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, p := range ps.store.Peers {
		if p.Name == name {
			updated := append([]Peer(nil), ps.store.Peers[:i]...)
			updated = append(updated, ps.store.Peers[i+1:]...)
			if err := ps.saveData(peerStoreData{Peers: updated}); err != nil {
				return err
			}
			ps.store.Peers = updated
			return nil
		}
	}
	return fmt.Errorf("peer %q not found", name)
}

// Get returns a peer by name
func (ps *PeerStore) Get(name string) *Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for _, p := range ps.store.Peers {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// GetByPublicKey returns a peer by public key
func (ps *PeerStore) GetByPublicKey(publicKey string) *Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for _, p := range ps.store.Peers {
		if p.PublicKey == publicKey {
			return &p
		}
	}
	return nil
}

// List returns all known peers
func (ps *PeerStore) List() []Peer {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]Peer, len(ps.store.Peers))
	copy(result, ps.store.Peers)
	return result
}

// save writes the peer store to disk (must be called with lock held)
func (ps *PeerStore) save() error {
	return ps.saveData(ps.store)
}

func (ps *PeerStore) saveData(store peerStoreData) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal peers: %w", err)
	}
	dir := filepath.Dir(ps.path)
	temp, err := os.CreateTemp(dir, ".peers-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary peers file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("set temporary peers permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary peers file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary peers file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary peers file: %w", err)
	}
	if err := os.Rename(tempPath, ps.path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace peers file: %w", err)
	}
	return nil
}

// ValidateStoredPeers inspects the persisted Peer store without migrating or
// rewriting it and returns the number of valid records.
func ValidateStoredPeers() (int, error) {
	dir, err := configDir()
	if err != nil {
		return 0, err
	}
	path := filepath.Join(dir, "peers.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read peers %s: %w", path, err)
	}
	var stored legacyPeerStoreData
	if err := json.Unmarshal(data, &stored); err != nil {
		return 0, fmt.Errorf("parse peers %s: %w", path, err)
	}
	peers := make([]Peer, 0, len(stored.Peers))
	for _, peer := range stored.Peers {
		peers = append(peers, Peer{
			Name: peer.Name, PublicKey: peer.PublicKey, PairedAt: peer.PairedAt,
		})
	}
	if err := validatePeerRecords(peers); err != nil {
		return 0, fmt.Errorf("validate peers %s: %w", path, err)
	}
	return len(peers), nil
}
