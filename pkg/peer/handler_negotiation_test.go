package peer

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func newNegotiationTestPeer(t *testing.T, timeout time.Duration) (*websocket.Conn, *Manager, *identity.Identity) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	hub, err := identity.Generate("hub")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := identity.Generate("agent")
	if err != nil {
		t.Fatal(err)
	}
	store, err := identity.NewPeerStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(identity.Peer{
		Name: agent.Name, PublicKey: agent.PublicKey, PairedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(hub, store, nil)
	handler := NewHandler(manager, store, toolevents.NewTracker(), identity.NewPairingManager(), NewPTYRelay(), "")
	handler.handshakeTimeout = timeout
	server := httptest.NewServer(http.HandlerFunc(handler.HandlePeer))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var challengeMessage Message
	if err := conn.ReadJSON(&challengeMessage); err != nil {
		t.Fatal(err)
	}
	var challenge ChallengePayload
	if err := json.Unmarshal(challengeMessage.Payload, &challenge); err != nil {
		t.Fatal(err)
	}
	challengeBytes, _ := base64.StdEncoding.DecodeString(challenge.Challenge)
	signature, err := agent.Sign(challengeBytes)
	if err != nil {
		t.Fatal(err)
	}
	auth, _ := NewMessage(MsgAuth, AuthPayload{
		PublicKey: agent.PublicKey, Signature: base64.StdEncoding.EncodeToString(signature),
	})
	if err := conn.WriteJSON(auth); err != nil {
		t.Fatal(err)
	}
	var authResult Message
	if err := conn.ReadJSON(&authResult); err != nil || authResult.Type != MsgAuthOK {
		t.Fatalf("auth result=%q err=%v", authResult.Type, err)
	}
	return conn, manager, agent
}

func TestNegotiationRejectsRuntimeMessageBeforeActivation(t *testing.T) {
	conn, manager, agent := newNegotiationTestPeer(t, time.Second)
	stateMessage, _ := NewMessage(MsgStateUpdate, StateUpdatePayload{})
	if err := conn.WriteJSON(stateMessage); err != nil {
		t.Fatal(err)
	}
	var response Message
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response.Type != MsgRuntimeError {
		t.Fatalf("response type=%q", response.Type)
	}
	if manager.GetPeerConnection(agent.Fingerprint()) != nil {
		t.Fatal("peer was activated before hello")
	}
}

func TestNegotiationRejectsLegacyPeerThatOmitsHello(t *testing.T) {
	conn, manager, agent := newNegotiationTestPeer(t, 40*time.Millisecond)
	var response Message
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response.Type != MsgRuntimeError {
		t.Fatalf("response type=%q", response.Type)
	}
	var runtimeError RuntimeError
	if err := json.Unmarshal(response.Payload, &runtimeError); err != nil ||
		runtimeError.Code != ErrorProtocolIncompatible {
		t.Fatalf("runtime error=%#v err=%v", runtimeError, err)
	}
	if manager.GetPeerConnection(agent.Fingerprint()) != nil {
		t.Fatal("legacy peer was activated")
	}
}
