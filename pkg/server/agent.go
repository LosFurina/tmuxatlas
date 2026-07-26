package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

// RunAgentSocket serves only the local Unix socket used by notify hooks. It
// never opens a TCP listener.
func RunAgentSocket(ctx context.Context, socketPath string, tracker *toolevents.Tracker, peerMgr *peer.Manager) error {
	if socketPath == "" {
		socketPath = socket.DefaultPath()
	}
	listener, err := socket.Listen(socketPath)
	if err != nil {
		return fmt.Errorf("listen on agent socket: %w", err)
	}
	defer func() {
		listener.Close()
		_ = socket.Cleanup(socketPath)
	}()

	instanceID, err := state.NewInstanceID()
	if err != nil {
		return fmt.Errorf("create agent instance ID: %w", err)
	}
	httpServer := &http.Server{
		Handler:           newLocalRouter(tracker, peerMgr, nil, nil, nativeHealth("agent", instanceID, true)),
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
