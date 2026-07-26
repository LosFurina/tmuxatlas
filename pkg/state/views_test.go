package state

import (
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

func TestProjectionViewsAreStableAndDefensive(t *testing.T) {
	operations, err := ReconcileHosts(NewProjection(), []HostSnapshot{
		{ID: "host-b", DisplayName: "same", Online: true, Sessions: []*tmux.Session{{ID: "$2", Name: "work"}}},
		{ID: "host-a", DisplayName: "same", Online: true, Sessions: []*tmux.Session{{
			ID: "$1", Name: "work", Windows: []*tmux.Window{{
				ID: "@1", Index: 0, Panes: []*tmux.Pane{{ID: "%1", Index: 0}},
			}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := NewProjection()
	for _, operation := range operations {
		if err := applyOperation(&projection, operation); err != nil {
			t.Fatal(err)
		}
	}
	sessions := projection.SessionViews()
	if len(sessions) != 2 || sessions[0].Host != "host-a" || sessions[1].Host != "host-b" {
		t.Fatalf("sessions = %+v", sessions)
	}
	sessions[0].Name = "mutated"
	if projection.Sessions[NewSessionKey("host-a", "work")].Name != "work" {
		t.Fatal("view mutation changed projection")
	}
	hosts := projection.HostViews()
	if len(hosts) != 2 || hosts[0].ID != "host-a" || hosts[1].ID != "host-b" {
		t.Fatalf("hosts = %+v", hosts)
	}
}
