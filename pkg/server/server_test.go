package server

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

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
			PublicURL:       "http://" + address,
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

func TestNoAuthAllowsAlternateLocalHostForLANPublicURL(t *testing.T) {
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
			PublicURL:       "http://10.0.0.10:7654",
			SocketPath:      filepath.Join(t.TempDir(), "tmuxatlas.sock"),
			Client:          tmuxClient,
			StateMgr:        state.NewManager(tmuxClient),
			Tracker:         toolevents.NewTracker(),
			ActivityTracker: activity.NewTracker(),
			AuthEnabled:     false,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get("http://" + address + "/api/version")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("no-auth alternate host returned %s", resp.Status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no-auth server did not start: %v", err)
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
		t.Fatal("no-auth server did not shut down")
	}
}

func TestNoAuthAllowsWebSocketFromAlternateLocalOriginForLANPublicURL(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	tmuxClient := &tmux.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, &Options{
			ListenAddress:   address,
			PublicURL:       "http://10.0.0.10:7654",
			SocketPath:      filepath.Join(tempDir, "tmuxatlas.sock"),
			Client:          tmuxClient,
			StateMgr:        state.NewManager(tmuxClient),
			Tracker:         toolevents.NewTracker(),
			ActivityTracker: activity.NewTracker(),
			AuthEnabled:     false,
		})
	}()

	endpoint := "ws://" + address + "/ws/events?schema=1"
	headers := http.Header{"Origin": {"http://" + address}}
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, response, err := websocket.DefaultDialer.Dial(endpoint, headers)
		if err == nil {
			conn.Close()
			break
		}
		if response != nil {
			response.Body.Close()
		}
		if time.Now().After(deadline) {
			cancel()
			<-result
			t.Fatalf("no-auth WebSocket from alternate local origin failed: %v", err)
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
		t.Fatal("no-auth WebSocket server did not shut down")
	}
}

func TestPublicOriginDoesNotExposeLocalToolEventIngest(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "tmuxatlas-router-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()

	tracker := toolevents.NewTracker()
	tmuxClient := &tmux.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, &Options{
			ListenAddress:   address,
			PublicURL:       "http://" + address,
			SocketPath:      filepath.Join(tempDir, "tmuxatlas.sock"),
			Client:          tmuxClient,
			StateMgr:        state.NewManager(tmuxClient),
			Tracker:         tracker,
			ActivityTracker: activity.NewTracker(),
		})
	}()
	defer func() {
		cancel()
		<-result
	}()

	var response *http.Response
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		response, err = http.Post("http://"+address+"/api/tool-event", "application/json",
			bytes.NewBufferString(`{"tool":"codex","status":"waiting","session":"public"}`))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("public tool-event status = %d, want 405", response.StatusCode)
	}
	if events := tracker.GetForSession("public"); len(events) != 0 {
		t.Fatalf("public request recorded %d events", len(events))
	}

	unixClient := &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("unix", filepath.Join(tempDir, "tmuxatlas.sock"))
		},
	}}
	response, err = unixClient.Post("http://localhost/api/tool-event", "application/json",
		bytes.NewBufferString(`{"tool":"codex","status":"waiting","session":"local"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("local tool-event status = %d, want 204", response.StatusCode)
	}
	if events := tracker.GetForSession("local"); len(events) != 1 {
		t.Fatalf("local request recorded %d events, want 1", len(events))
	}
}
