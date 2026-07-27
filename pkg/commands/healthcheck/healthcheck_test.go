package healthcheck

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

func TestProbeUnixHealth(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "health.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"role":"hub","deployment":"docker","version":"v1.2.3","commit":"abc","ready":true}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		_ = listener.Close()
	})

	health, err := probe(t.Context(), socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if health.Role != "hub" || health.Deployment != "docker" || !health.Ready {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func TestProbeRejectsNotReady(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "health.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"role":"hub","deployment":"docker","ready":false}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		_ = listener.Close()
	})

	if _, err := probe(t.Context(), socketPath); err == nil {
		t.Fatal("expected a not-ready probe to fail")
	}
}
