package state

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCoordinatorMaterialCommitAndNoOp(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()
	host := Host{Key: NewHostKey("host-a"), ID: "host-a", DisplayName: "alpha"}
	operation := Operation{Kind: OperationUpsertHost, Host: &host}

	first, err := coordinator.Commit(ctx, operation)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed || first.Revision != 1 {
		t.Fatalf("first commit = %+v", first)
	}
	second, err := coordinator.Commit(ctx, operation)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.Revision != 1 {
		t.Fatalf("no-op commit = %+v", second)
	}
}

func TestCoordinatorSerializesConcurrentProducers(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()
	const producers = 64

	var wg sync.WaitGroup
	revisions := make(chan uint64, producers)
	for index := 0; index < producers; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			hostID := string(rune('a' + index))
			host := Host{Key: NewHostKey(hostID), ID: hostID}
			result, err := coordinator.Commit(ctx, Operation{Kind: OperationUpsertHost, Host: &host})
			if err != nil {
				t.Errorf("commit: %v", err)
				return
			}
			revisions <- result.Revision
		}(index)
	}
	wg.Wait()
	close(revisions)

	seen := make(map[uint64]bool, producers)
	for revision := range revisions {
		if seen[revision] {
			t.Fatalf("duplicate committed revision %d", revision)
		}
		seen[revision] = true
	}
	for revision := uint64(1); revision <= producers; revision++ {
		if !seen[revision] {
			t.Fatalf("missing committed revision %d", revision)
		}
	}
}

func TestCoordinatorSnapshotIsDefensive(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()
	health := Health{
		HostKey: NewHostKey("host-a"),
		Facts:   map[string]any{"checks": []any{"ok"}},
	}
	if _, err := coordinator.Commit(ctx, Operation{Kind: OperationUpsertHealth, Health: &health}); err != nil {
		t.Fatal(err)
	}
	first, err := coordinator.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first.State.Health[health.HostKey].Facts["checks"].([]any)[0] = "mutated"
	delete(first.State.Health, health.HostKey)

	second, err := coordinator.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := second.State.Health[health.HostKey]; !ok {
		t.Fatal("caller mutation changed coordinator projection")
	}
	if got := second.State.Health[health.HostKey].Facts["checks"].([]any)[0]; got != "ok" {
		t.Fatalf("nested caller mutation leaked into snapshot: %v", got)
	}
}

func TestCoordinatorAtomicSubscribeHasNoGap(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()
	subscription, err := coordinator.Subscribe(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(subscription.Cancel)

	host := Host{Key: NewHostKey("host-a"), ID: "host-a"}
	result, err := coordinator.Commit(ctx, Operation{Kind: OperationUpsertHost, Host: &host})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-subscription.C:
		if message.Delta == nil {
			t.Fatalf("message = %+v", message)
		}
		if message.Delta.BaseRevision != subscription.Snapshot.Revision ||
			message.Delta.Revision != result.Revision {
			t.Fatalf("snapshot revision %d, delta = %+v", subscription.Snapshot.Revision, message.Delta)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first delta")
	}
}

func TestCoordinatorSlowSubscriberGetsResync(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()
	subscription, err := coordinator.Subscribe(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	for _, hostID := range []string{"host-a", "host-b"} {
		host := Host{Key: NewHostKey(hostID), ID: hostID}
		if _, err := coordinator.Commit(ctx, Operation{Kind: OperationUpsertHost, Host: &host}); err != nil {
			t.Fatal(err)
		}
	}
	message, ok := <-subscription.C
	if !ok {
		t.Fatal("subscription closed before resync outcome")
	}
	if message.Outcome == nil || message.Outcome.Type != EnvelopeResyncRequired {
		t.Fatalf("message = %+v, want resync-required", message)
	}
	if _, ok := <-subscription.C; ok {
		t.Fatal("overflowed subscription should close after resync outcome")
	}
}

func TestCoordinatorProcessGeneration(t *testing.T) {
	first, err := NewCoordinator()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCoordinator()
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)
	if first.InstanceID() == second.InstanceID() {
		t.Fatalf("separate process coordinators reused instance %q", first.InstanceID())
	}
	if !first.SupportsSchema(SchemaVersion) || first.SupportsSchema(SchemaVersion+1) {
		t.Fatal("schema compatibility check returned an invalid result")
	}
}
