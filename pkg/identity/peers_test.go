package identity

import (
	"encoding/json"
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
	legacy := `{
  "peers": [{
    "name": "remote",
    "public_key": "public-key",
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
	if len(peers) != 1 || peers[0].Name != "remote" || peers[0].PublicKey != "public-key" || !peers[0].PairedAt.Equal(pairedAt) {
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
