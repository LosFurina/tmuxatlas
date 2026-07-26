package state

import (
	"context"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

func TestReplaceHostsProjectsSameNamedSessionsIndependently(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	session := func(id string) *tmux.Session {
		return &tmux.Session{
			ID: id, Name: "work",
			Windows: []*tmux.Window{{
				ID: "@" + id, Name: "editor",
				Panes: []*tmux.Pane{{ID: "%" + id}},
			}},
		}
	}
	result, err := coordinator.ReplaceHosts(context.Background(), []HostSnapshot{
		{ID: "host-a", DisplayName: "duplicate", Online: true, Sessions: []*tmux.Session{session("1")}},
		{ID: "host-b", DisplayName: "duplicate", Online: true, Sessions: []*tmux.Session{session("2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Revision != 1 {
		t.Fatalf("commit = %+v", result)
	}
	snapshot, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.State.Hosts) != 2 || len(snapshot.State.Sessions) != 2 {
		t.Fatalf("projection hosts=%d sessions=%d", len(snapshot.State.Hosts), len(snapshot.State.Sessions))
	}
	if _, ok := snapshot.State.Sessions[NewSessionKey("host-a", "work")]; !ok {
		t.Fatal("host-a/work missing")
	}
	if _, ok := snapshot.State.Sessions[NewSessionKey("host-b", "work")]; !ok {
		t.Fatal("host-b/work missing")
	}
}

func TestReplaceHostsNoOpDoesNotAdvanceRevision(t *testing.T) {
	coordinator := NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	hosts := []HostSnapshot{{ID: "host-a", Online: true}}
	if _, err := coordinator.ReplaceHosts(context.Background(), hosts); err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.ReplaceHosts(context.Background(), hosts)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.Revision != 1 {
		t.Fatalf("second projection = %+v", result)
	}
}
