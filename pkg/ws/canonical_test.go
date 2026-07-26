package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LosFurina/tmuxatlas/pkg/state"
)

func canonicalTestServer(t *testing.T, coordinator *state.Coordinator) (*httptest.Server, string) {
	t.Helper()
	hub := NewHub(nil, nil)
	hub.SetCoordinator(coordinator)
	server := httptest.NewServer(http.HandlerFunc(hub.HandleEvents))
	t.Cleanup(server.Close)
	return server, "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestCanonicalHandlerSendsAtomicSnapshotThenDelta(t *testing.T) {
	coordinator := state.NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	_, endpoint := canonicalTestServer(t, coordinator)

	conn, _, err := websocket.DefaultDialer.Dial(endpoint+"?schema=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot state.SnapshotEnvelope
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != state.EnvelopeSnapshot || snapshot.InstanceID != "instance-a" {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	host := state.Host{Key: state.NewHostKey("host-a"), ID: "host-a"}
	result, err := coordinator.Commit(context.Background(), state.Operation{
		Kind: state.OperationUpsertHost, Host: &host,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var delta state.DeltaEnvelope
	if err := json.Unmarshal(data, &delta); err != nil {
		t.Fatal(err)
	}
	if delta.Type != state.EnvelopeDelta ||
		delta.BaseRevision != snapshot.Revision ||
		delta.Revision != result.Revision {
		t.Fatalf("delta = %+v after snapshot %+v", delta, snapshot)
	}
}

func TestCanonicalHandlerReturnsReloadRequiredForSchemaMismatch(t *testing.T) {
	coordinator := state.NewCoordinatorWithInstanceID("instance-a")
	t.Cleanup(coordinator.Close)
	_, endpoint := canonicalTestServer(t, coordinator)

	for _, suffix := range []string{"", "?schema=999"} {
		t.Run(suffix, func(t *testing.T) {
			conn, _, err := websocket.DefaultDialer.Dial(endpoint+suffix, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(time.Second))
			_, data, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			var outcome state.OutcomeEnvelope
			if err := json.Unmarshal(data, &outcome); err != nil {
				t.Fatal(err)
			}
			if outcome.Type != state.EnvelopeReloadRequired {
				t.Fatalf("outcome = %+v", outcome)
			}
		})
	}
}
