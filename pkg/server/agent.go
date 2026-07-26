package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

// RunAgentSocket serves only the local Unix socket used by notify hooks. It
// never opens a TCP listener.
func RunAgentSocket(ctx context.Context, socketPath string, tracker *toolevents.Tracker, peerMgr *peer.Manager) error {
	if socketPath == "" {
		socketPath = socket.DefaultPath()
	}
	if err := socket.EnsureDir(socketPath); err != nil {
		return fmt.Errorf("create agent socket directory: %w", err)
	}
	if err := socket.Cleanup(socketPath); err != nil {
		return fmt.Errorf("clean stale agent socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on agent socket: %w", err)
	}
	defer func() {
		listener.Close()
		_ = socket.Cleanup(socketPath)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/tool-event", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var event toolevents.Event
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if event.Tool == "" || event.Status == "" || event.Session == "" {
			http.Error(w, "tool, status, and session are required", http.StatusBadRequest)
			return
		}
		if event.Host == "" && peerMgr != nil {
			event.Host = peerMgr.LocalID()
			event.HostName = peerMgr.LocalName()
		}
		tracker.Record(&event)
		w.WriteHeader(http.StatusNoContent)
	})

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	logrus.WithField("socket", socketPath).Info("agent listening on local Unix socket")
	select {
	case <-ctx.Done():
	case err := <-serverErr:
		return fmt.Errorf("agent socket: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
