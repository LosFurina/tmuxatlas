package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPeerStoreMigratesLegacyCertificateTrust(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")
	pairedAt := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	publicKey := testPublicKey(t)
	legacy := `{
  "peers": [{
    "name": "remote",
    "public_key": "` + publicKey + `",
    "paired_at": "` + pairedAt.Format(time.RFC3339) + `",
    "tls_cert_pem": "legacy-leaf",
    "ca_cert_pem": "legacy-ca"
  }]
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := loadPeerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	peers := store.List()
	if len(peers) != 1 || peers[0].Name != "remote" || peers[0].PublicKey != publicKey || !peers[0].PairedAt.Equal(pairedAt) {
		t.Fatalf("identity was not preserved: %#v", peers)
	}

	backup, err := os.ReadFile(path + ".pre-system-trust.bak")
	if err != nil {
		t.Fatalf("read migration backup: %v", err)
	}
	if string(backup) != legacy {
		t.Error("migration backup does not match original peer store")
	}

	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migrated), "cert_pem") || strings.Contains(string(migrated), "legacy-ca") {
		t.Fatalf("legacy trust remains after migration: %s", migrated)
	}
	var decoded peerStoreData
	if err := json.Unmarshal(migrated, &decoded); err != nil {
		t.Fatal(err)
	}

	if _, err := loadPeerStore(path); err != nil {
		t.Fatalf("idempotent reload failed: %v", err)
	}
	backupAgain, err := os.ReadFile(path + ".pre-system-trust.bak")
	if err != nil || string(backupAgain) != legacy {
		t.Fatal("idempotent reload replaced the original backup")
	}
}

func testPublicKey(t *testing.T) string {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(publicKey)
}

func TestLoadPeerStoreRejectsMalformedAndDuplicateRecordsWithoutRewriting(t *testing.T) {
	tests := []struct {
		name  string
		peers []Peer
		match string
	}{
		{
			name:  "malformed key",
			peers: []Peer{{Name: "one", PublicKey: "bad"}},
			match: "peers[0]",
		},
		{
			name: "duplicate name",
			peers: []Peer{
				{Name: "same", PublicKey: testPublicKey(t)},
				{Name: "same", PublicKey: testPublicKey(t)},
			},
			match: "duplicate name",
		},
	}
	duplicateKey := testPublicKey(t)
	tests = append(tests, struct {
		name  string
		peers []Peer
		match string
	}{
		name: "duplicate key",
		peers: []Peer{
			{Name: "one", PublicKey: duplicateKey},
			{Name: "two", PublicKey: duplicateKey},
		},
		match: "duplicate public key",
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "peers.json")
			raw, err := json.Marshal(peerStoreData{Peers: test.peers})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadPeerStore(path); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("loadPeerStore error = %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(raw) {
				t.Fatal("invalid Peer store was rewritten")
			}
		})
	}
}

func TestPeerStoreAddRejectsIdentityConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	store, err := loadPeerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first := Peer{Name: "one", PublicKey: testPublicKey(t), PairedAt: time.Now()}
	if err := store.Add(first); err != nil {
		t.Fatal(err)
	}
	for _, peer := range []Peer{
		{Name: "two", PublicKey: first.PublicKey, PairedAt: time.Now()},
		{Name: "one", PublicKey: testPublicKey(t), PairedAt: time.Now()},
	} {
		if err := store.Add(peer); !errors.Is(err, ErrPeerConflict) {
			t.Fatalf("Add(%+v) error = %v", peer, err)
		}
	}
	if got := store.List(); len(got) != 1 || got[0] != first {
		t.Fatalf("store changed after conflicts: %#v", got)
	}
}
