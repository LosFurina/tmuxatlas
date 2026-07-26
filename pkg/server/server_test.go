package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func TestHTTPOriginDoesNotTouchLegacyTLSFiles(t *testing.T) {
	tempDir := t.TempDir()
	legacyFiles := []string{"ca-cert.pem", "ca-key.pem", "server-cert.pem", "server-key.pem"}
	for _, name := range legacyFiles {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("legacy-sentinel"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	tmuxClient := &tmux.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, &Options{
			ListenAddress:   address,
			PublicURL:       "http://localhost:7654",
			SocketPath:      filepath.Join(tempDir, "tmuxatlas.sock"),
			Client:          tmuxClient,
			StateMgr:        state.NewManager(tmuxClient),
			Tracker:         toolevents.NewTracker(),
			ActivityTracker: activity.NewTracker(),
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get("http://" + address + "/api/version")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("version endpoint returned %s", resp.Status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("HTTP origin did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP origin did not shut down")
	}

	for _, name := range legacyFiles {
		data, err := os.ReadFile(filepath.Join(tempDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "legacy-sentinel" {
			t.Fatalf("legacy TLS file %s was modified", name)
		}
	}
}
