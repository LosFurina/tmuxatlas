package state

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func TestProducerAdaptersCommitOrderedDeltas(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	ctx := context.Background()

	waiting := &toolevents.Event{
		Tool: toolevents.ToolCodex, Status: toolevents.StatusWaiting,
		Session: "work", Window: 1, Pane: "%1",
	}
	first, err := coordinator.ApplyToolEvent(ctx, waiting, "host-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.ReplaceActivity(ctx, []*activity.Snapshot{{
		Host: "host-a", SessionName: "work", Sparkline: []int64{1}, TotalBytes: 1,
	}}, "host-a")
	if err != nil {
		t.Fatal(err)
	}
	third, err := coordinator.SetMetadata(ctx, "preferences", map[string]any{"theme": "blue"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || second.Revision != 2 || third.Revision != 3 {
		t.Fatalf("producer revisions = %d, %d, %d", first.Revision, second.Revision, third.Revision)
	}

	snapshot, err := coordinator.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.State.ToolEvents) != 1 || len(snapshot.State.Activity) != 1 {
		t.Fatalf("projection = %+v", snapshot.State)
	}
	var metadata map[string]string
	if err := json.Unmarshal(snapshot.State.Metadata["preferences"], &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["theme"] != "blue" {
		t.Fatalf("metadata = %+v", metadata)
	}

	completed := *waiting
	completed.Status = toolevents.StatusCompleted
	if _, err := coordinator.ApplyToolEvent(ctx, &completed, "host-a"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = coordinator.Snapshot(ctx)
	if len(snapshot.State.ToolEvents) != 0 {
		t.Fatal("completed event did not clear canonical tool event")
	}
}
