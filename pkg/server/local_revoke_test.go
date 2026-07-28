package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func TestLocalRuntimeRevokeCommitsAndClosesActivePeer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	hub, _ := identity.Generate("hub")
	agent, _ := identity.Generate("agent")
	store, err := identity.NewPeerStore()
	if err != nil {
		t.Fatal(err)
	}
	record := identity.Peer{Name: agent.Name, PublicKey: agent.PublicKey, PairedAt: time.Now()}
	if err := store.Add(record); err != nil {
		t.Fatal(err)
	}
	manager := peer.NewManager(hub, store, nil)
	manager.RegisterPeer(record.Fingerprint(), record.Name, record.PublicKey, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/peers/revoke",
		bytes.NewBufferString(`{"name":"agent"}`))
	response := httptest.NewRecorder()
	newLocalRouter(toolevents.NewTracker(), manager, nil, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if store.Get(record.Name) != nil || manager.HasHost(record.Fingerprint()) {
		t.Fatal("local runtime revoke did not update store and registry")
	}
}
