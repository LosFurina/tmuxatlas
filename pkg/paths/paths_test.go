package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDirMigratesLegacyDataWithoutDeletingSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, ".config", legacyAppName)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyFile := filepath.Join(legacyDir, "identity.json")
	if err := os.WriteFile(legacyFile, []byte("legacy-identity"), 0o600); err != nil {
		t.Fatal(err)
	}

	targetDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if targetDir != filepath.Join(home, ".config", AppName) {
		t.Fatalf("ConfigDir() = %q", targetDir)
	}
	migrated, err := os.ReadFile(filepath.Join(targetDir, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(migrated) != "legacy-identity" {
		t.Fatalf("migrated data = %q", migrated)
	}
	if _, err := os.Stat(legacyFile); err != nil {
		t.Fatalf("legacy rollback source was removed: %v", err)
	}
}

func TestConfigDirUsesXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, AppName); dir != want {
		t.Fatalf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestConfigDirDoesNotOverwriteNewData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyDir := filepath.Join(home, ".config", legacyAppName)
	targetDir := filepath.Join(home, ".config", AppName)
	for _, dir := range []string{legacyDir, targetDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "auth.json"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "auth.json"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ConfigDir(); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(targetDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "current" {
		t.Fatalf("current data was overwritten: %q", current)
	}
}
