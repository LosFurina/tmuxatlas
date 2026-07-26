package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvUsesConfigFileWithoutOverridingEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUXATLAS_LISTEN", "127.0.0.1:9999")
	previousPublicURL, hadPublicURL := os.LookupEnv("TMUXATLAS_PUBLIC_URL")
	if err := os.Unsetenv("TMUXATLAS_PUBLIC_URL"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadPublicURL {
			_ = os.Setenv("TMUXATLAS_PUBLIC_URL", previousPublicURL)
		} else {
			_ = os.Unsetenv("TMUXATLAS_PUBLIC_URL")
		}
	})
	configDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	content := "TMUXATLAS_PUBLIC_URL=https://tmux.example.com\nTMUXATLAS_LISTEN=127.0.0.1:7654\n"
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("TMUXATLAS_PUBLIC_URL"); got != "https://tmux.example.com" {
		t.Fatalf("TMUXATLAS_PUBLIC_URL = %q", got)
	}
	if got := os.Getenv("TMUXATLAS_LISTEN"); got != "127.0.0.1:9999" {
		t.Fatalf("existing environment was overridden: %q", got)
	}
}

func TestLoadEnvRejectsUnrelatedVariables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte("PATH=/tmp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnv(); err == nil {
		t.Fatal("expected unrelated variable to be rejected")
	}
}
