package agentcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExecutableDiscoversUserBinsWithoutPATH(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{
		filepath.Join(home, ".nvm", "versions", "node", "v24.0.0", "bin", "opencode"),
		filepath.Join(home, ".local", "bin", "agy"),
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", "")
	if _, ok := FindExecutable("opencode", home); !ok {
		t.Fatal("opencode in NVM bin was not discovered")
	}
	if _, ok := FindExecutable("agy", home); !ok {
		t.Fatal("agy in .local/bin was not discovered")
	}
}

func TestFindExecutableRejectsNonExecutableAndDirectories(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(filepath.Join(bin, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "not-executable"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	for _, name := range []string{"directory", "not-executable", "missing"} {
		if _, ok := FindExecutable(name, home); ok {
			t.Fatalf("%s must not be discovered", name)
		}
	}
}

func TestAgentStatusReportsAgyAsPassive(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".local", "bin", "agy")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	for _, status := range checkAgents(home).Agents {
		if status.Key == "agy" {
			if !status.Installed || status.Configured || status.TrackingMode != "passive" || status.SetupRequired {
				t.Fatalf("unexpected Agy status: %#v", status)
			}
			return
		}
	}
	t.Fatal("Agy status missing")
}
