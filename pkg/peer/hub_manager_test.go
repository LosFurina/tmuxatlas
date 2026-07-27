package peer

import (
	"context"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
)

func TestHubManagerDoesNotRegisterOrRouteLocalHost(t *testing.T) {
	node, err := identity.Generate("hub")
	if err != nil {
		t.Fatal(err)
	}
	manager := NewHubManager(node, nil)
	if manager.IsLocal(manager.LocalID()) {
		t.Fatal("remote-only Hub identity was marked as a local tmux host")
	}
	if manager.HasHost(manager.LocalID()) || len(manager.GetHosts()) != 0 {
		t.Fatal("remote-only Hub registered a synthetic local host")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.RunContext(ctx)
		close(done)
	}()
	cancel()
	<-done
}
