package peer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
)

func newAuthorizedManager(t *testing.T) (*Manager, *identity.PeerStore, *identity.Peer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	authorized := &identity.Peer{Name: agent.Name, PublicKey: agent.PublicKey, PairedAt: time.Now()}
	if err := store.Add(*authorized); err != nil {
		t.Fatal(err)
	}
	return NewManager(hub, store, nil), store, authorized
}

func TestLiveRevokeClosesGenerationRequestsPTYAndRejectsReconnect(t *testing.T) {
	manager, store, authorized := newAuthorizedManager(t)
	fingerprint := authorized.Fingerprint()
	connection := newPeerConnection(context.Background(), fingerprint, 1, RuntimeCapabilities, "instance", 8,
		func(*Message) error { return nil }, nil)
	if !manager.ActivateAuthorizedPeer(fingerprint, authorized.Name, authorized.PublicKey, connection) {
		t.Fatal("authorized peer did not activate")
	}
	request := validRuntimeRequest("pending-revoke", 1)
	request.Target.HostID = fingerprint
	outcomes, err := connection.requests.Register(request)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewPTYRelay().RegisterPending(connection,
		SessionTarget{HostID: fingerprint, Session: "work"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RevokePeer(authorized.Name); err != nil {
		t.Fatal(err)
	}
	if store.Get(authorized.Name) != nil || manager.GetPeerConnection(fingerprint) != nil {
		t.Fatal("revoked peer remained authorized or active")
	}
	select {
	case outcome := <-outcomes:
		if outcome.Error == nil || outcome.Error.Code != ErrorPeerRevoked {
			t.Fatalf("pending outcome=%#v", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request was not completed")
	}
	select {
	case <-owner.ctx.Done():
	default:
		t.Fatal("owned PTY was not torn down")
	}
	reconnect := newPeerConnection(context.Background(), fingerprint, 2, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	if manager.ActivateAuthorizedPeer(fingerprint, authorized.Name, authorized.PublicKey, reconnect) {
		t.Fatal("revoked identity reconnected without restart")
	}
}

func TestLiveRevokePersistenceFailureKeepsRuntimeAuthorization(t *testing.T) {
	manager, store, authorized := newAuthorizedManager(t)
	fingerprint := authorized.Fingerprint()
	connection := newPeerConnection(context.Background(), fingerprint, 1, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	if !manager.ActivateAuthorizedPeer(fingerprint, authorized.Name, authorized.PublicKey, connection) {
		t.Fatal("activate")
	}
	storePath := filepath.Join(os.Getenv("HOME"), ".config", "tmuxatlas", "peers.json")
	if err := os.Remove(storePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(storePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RevokePeer(authorized.Name); err == nil {
		t.Fatal("revocation unexpectedly succeeded")
	}
	if store.Get(authorized.Name) == nil || manager.GetPeerConnection(fingerprint) != connection {
		t.Fatal("persistence failure partially revoked runtime state")
	}
}

func TestReplacementWithNewAgentInstanceCompletesExecutionUnknown(t *testing.T) {
	manager := newRouterManager(t)
	first := newPeerConnection(context.Background(), "peer", 1, RuntimeCapabilities, "instance-a", 1,
		func(*Message) error { return nil }, nil)
	if !manager.ActivatePeer("peer", "peer", "key", first) {
		t.Fatal("activate first")
	}
	request := validRuntimeRequest("unknown", 1)
	outcomes, _ := first.requests.Register(request)
	second := newPeerConnection(context.Background(), "peer", 2, RuntimeCapabilities, "instance-b", 1,
		func(*Message) error { return nil }, nil)
	if !manager.ActivatePeer("peer", "peer", "key", second) {
		t.Fatal("activate second")
	}
	outcome := <-outcomes
	if outcome.Error == nil || outcome.Error.Code != ErrorExecutionUnknown {
		encoded, _ := json.Marshal(outcome)
		t.Fatalf("replacement outcome=%s", encoded)
	}
	if !errors.Is(first.Send(context.Background(), &Message{}), ErrPeerOffline) {
		t.Fatal("old generation still accepts sends")
	}
}
