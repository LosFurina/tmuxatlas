package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

	// Replace if public key already exists
	for i, p := range ps.store.Peers {
		if p.PublicKey == peer.PublicKey {
			ps.store.Peers[i] = peer
			return ps.save()
		}
	}

	ps.store.Peers = append(ps.store.Peers, peer)
	return ps.save()
}

// Remove removes a peer by name and persists to disk
func (ps *PeerStore) Remove(name string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, p := range ps.store.Peers {
		if p.Name == name {
			ps.store.Peers = append(ps.store.Peers[:i], ps.store.Peers[i+1:]...)
			return ps.save()
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
	data, err := json.MarshalIndent(ps.store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal peers: %w", err)
	}
	if err := os.WriteFile(ps.path, data, 0600); err != nil {
		return fmt.Errorf("write peers: %w", err)
	}
	return nil
}
