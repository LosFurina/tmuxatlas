package peer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
)

func TestGenerationReplacementAndCASCleanup(t *testing.T) {
	node, err := identity.Generate("hub")
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(node, nil, nil)
	var firstClosed atomic.Int32
	firstGeneration := manager.ReserveGeneration("peer")
	first := newPeerConnection(context.Background(), "peer", firstGeneration, RuntimeCapabilities, "instance-1", 1,
		func(*Message) error { return nil },
		func() error { firstClosed.Add(1); return nil },
	)
	if !manager.ActivatePeer("peer", "Peer", "key", first) {
		t.Fatal("first generation was not activated")
	}
	secondGeneration := manager.ReserveGeneration("peer")
	second := newPeerConnection(context.Background(), "peer", secondGeneration, RuntimeCapabilities, "instance-2", 1,
		func(*Message) error { return nil }, func() error { return nil },
	)
	if !manager.ActivatePeer("peer", "Peer", "key", second) {
		t.Fatal("second generation was not activated")
	}
	if firstClosed.Load() != 1 {
		t.Fatalf("replaced connection close count = %d", firstClosed.Load())
	}
	if manager.UnregisterPeerGeneration("peer", firstGeneration) {
		t.Fatal("old generation cleanup removed the new connection")
	}
	if got := manager.GetPeerConnection("peer"); got != second {
		t.Fatal("new generation is not active")
	}
	second.Close()
	first.Wait()
	second.Wait()
}

func TestRapidReconnectSendAndCleanupInterleaving(t *testing.T) {
	node, _ := identity.Generate("hub")
	manager := NewManager(node, nil, nil)
	var latest *PeerConnection
	for generation := uint64(1); generation <= 50; generation++ {
		connection := newPeerConnection(context.Background(), "peer", generation, RuntimeCapabilities, "instance", 2,
			func(*Message) error { return nil }, nil)
		if !manager.ActivatePeer("peer", "peer", "key", connection) {
			t.Fatalf("generation %d did not activate", generation)
		}
		if latest != nil {
			if manager.UnregisterPeerGeneration("peer", generation-1) {
				t.Fatalf("generation %d cleanup removed generation %d", generation-1, generation)
			}
		}
		latest = connection
	}
	if manager.GetPeerConnection("peer") != latest {
		t.Fatal("latest generation is not active")
	}

	blocked := make(chan struct{})
	release := make(chan struct{})
	old := newPeerConnection(context.Background(), "racing", 1, RuntimeCapabilities, "instance", 2,
		func(*Message) error {
			select {
			case blocked <- struct{}{}:
			default:
			}
			<-release
			return nil
		}, nil)
	if !manager.ActivatePeer("racing", "racing", "key", old) {
		t.Fatal("activate racing generation")
	}
	message, _ := NewMessage("race", nil)
	if err := old.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	<-blocked
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		err := old.Send(context.Background(), message)
		if err != nil && !errors.Is(err, ErrPeerOffline) && !errors.Is(err, ErrQueueFull) {
			t.Errorf("send race error=%v", err)
		}
	}()
	go func() {
		defer wait.Done()
		replacement := newPeerConnection(context.Background(), "racing", 2, RuntimeCapabilities, "instance", 2,
			func(*Message) error { return nil }, nil)
		if !manager.ActivatePeer("racing", "racing", "key", replacement) {
			t.Error("replacement did not activate")
		}
	}()
	wait.Wait()
	close(release)
	old.Wait()
	if manager.GetPeerConnection("racing").Generation != 2 {
		t.Fatal("send/replace race lost the replacement")
	}
}

func TestPeerConnectionSendErrorsAreExplicit(t *testing.T) {
	connection := newPeerConnection(context.Background(), "peer", 1, nil, "instance", 1,
		func(*Message) error { return nil }, nil)
	message, _ := NewMessage("test", nil)
	if err := connection.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := connection.Send(context.Background(), message); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("second send error = %v, want queue full", err)
	}
	connection.Close()
	if err := connection.Send(context.Background(), message); !errors.Is(err, ErrPeerOffline) {
		t.Fatalf("closed send error = %v, want peer offline", err)
	}
}
