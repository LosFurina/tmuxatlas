package peers

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/socket"
)

func TestRevokeViaHubAndOfflineDetection(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "ta-peers-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	missing := filepath.Join(tempDir, "missing.sock")
	online, err := revokeViaHub(context.Background(), missing, "agent")
	if err != nil || online {
		t.Fatalf("missing socket online=%v err=%v", online, err)
	}

	socketPath := filepath.Join(tempDir, "runtime", "hub.sock")
	listener, err := socket.Listen(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Cleanup(socketPath)
	defer listener.Close()
	received := make(chan struct{}, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/peers/revoke" || request.Method != http.MethodPost {
			t.Errorf("request=%s %s", request.Method, request.URL.Path)
		}
		received <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"agent","fingerprint":"abc","revoked":true}`))
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()

	online, err = revokeViaHub(context.Background(), socketPath, "agent")
	if err != nil || !online {
		t.Fatalf("running socket online=%v err=%v", online, err)
	}
	select {
	case <-received:
	default:
		t.Fatal("running Hub did not receive revoke")
	}
}

func TestRevokeViaHubDoesNotFallbackAfterHubRejects(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "ta-peers-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := filepath.Join(tempDir, "runtime", "hub.sock")
	listener, err := socket.Listen(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Cleanup(socketPath)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "persist failed", http.StatusConflict)
	})}
	defer server.Close()
	go func(listener net.Listener) { _ = server.Serve(listener) }(listener)
	online, err := revokeViaHub(context.Background(), socketPath, "agent")
	if !online || err == nil {
		t.Fatalf("rejected revoke online=%v err=%v", online, err)
	}
}
