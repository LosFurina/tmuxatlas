package server

import (
	"context"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/state"
)

func TestSyncFleetHealthKeepsDuplicateNamesAndUnknownFacts(t *testing.T) {
	coordinator := state.NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	now := time.Now()
	hosts := []peer.HostInfo{
		{ID: "host-a", Name: "duplicate", Online: true, Version: common.VERSION, RuntimeProtocol: 1, LastSeen: now},
		{ID: "host-b", Name: "duplicate", Online: false, Version: "", RuntimeProtocol: 1, LastSeen: now.Add(-time.Minute)},
	}
	if err := syncFleetHealth(context.Background(), coordinator, hosts, now); err != nil {
		t.Fatal(err)
	}
	snapshot, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.State.Health) != 2 {
		t.Fatalf("health records = %d", len(snapshot.State.Health))
	}
	first := snapshot.State.Health[state.NewHostKey("host-a")].Facts
	second := snapshot.State.Health[state.NewHostKey("host-b")].Facts
	if first["summary"] != "unknown" {
		t.Fatalf("host-a summary = %v, missing checks must remain unknown", first["summary"])
	}
	if second["summary"] != "offline" {
		t.Fatalf("host-b summary = %v", second["summary"])
	}
}
