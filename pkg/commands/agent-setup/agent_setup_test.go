package agentsetup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInstalledUsesHomeAwareFinder(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".nvm", "versions", "node", "v24", "bin", "opencode")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	if !detectInstalled("opencode", home) {
		t.Fatal("NVM OpenCode must be detected")
	}
}
