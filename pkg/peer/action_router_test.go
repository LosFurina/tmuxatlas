package peer

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

type recordingExecutor struct {
	calls atomic.Int32
}

func (executor *recordingExecutor) Execute(_ context.Context, operation string, target SessionTarget, _ json.RawMessage) (json.RawMessage, error) {
	executor.calls.Add(1)
	return json.Marshal(map[string]any{"operation": operation, "target": target})
}

func newRouterManager(t *testing.T) *Manager {
	t.Helper()
	identity, err := identity.Generate("hub")
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(identity, nil, nil)
}

func TestActionRouterRoutesExplicitLocalAndRemoteTargetsWithoutFallback(t *testing.T) {
	manager := newRouterManager(t)
	local := &recordingExecutor{}
	router := NewActionRouter(manager, local, time.Second)
	localTarget := SessionTarget{HostID: manager.LocalID(), Session: "same"}
	if _, err := router.Execute(context.Background(), "new", localTarget, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}

	remoteID := "remote-host"
	var connection *PeerConnection
	connection = newPeerConnection(context.Background(), remoteID, 1, RuntimeCapabilities, "agent", 4,
		func(message *Message) error {
			var request RuntimeRequest
			if err := json.Unmarshal(message.Payload, &request); err != nil {
				return err
			}
			result, _ := json.Marshal(map[string]any{
				"target": SessionTarget{HostID: remoteID, Session: "renamed"},
			})
			connectionResult := RuntimeResult{
				RequestID: request.RequestID, Generation: request.Generation, Result: result,
			}
			// Complete after Send has registered the request.
			go connection.requests.CompleteResult(connectionResult)
			return nil
		}, nil)
	if !manager.ActivatePeer(remoteID, "remote", "key", connection) {
		t.Fatal("activate remote")
	}
	manager.UpdatePeerSessions(remoteID, []*tmux.Session{{Name: "same"}})
	response, err := router.Execute(context.Background(), "rename",
		SessionTarget{HostID: remoteID, Session: "same"}, json.RawMessage(`{"new_name":"renamed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if local.calls.Load() != 1 {
		t.Fatalf("local executor calls=%d, remote route fell back locally", local.calls.Load())
	}
	var result struct {
		Target SessionTarget `json:"target"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil || result.Target.HostID != remoteID ||
		result.Target.Session != "renamed" {
		t.Fatalf("remote rename result=%s err=%v", response.Result, err)
	}

	manager.UnregisterPeerGeneration(remoteID, connection.Generation)
	_, err = router.Execute(context.Background(), "rename",
		SessionTarget{HostID: remoteID, Session: "same"}, json.RawMessage(`{"new_name":"never-local"}`))
	var runtimeError RuntimeError
	if !errors.As(err, &runtimeError) || runtimeError.Code != ErrorPeerOffline {
		t.Fatalf("offline route error=%v", err)
	}
	if local.calls.Load() != 1 {
		t.Fatal("offline remote action executed locally")
	}
}

func TestActionRouterRejectsMissingAndUnknownHost(t *testing.T) {
	manager := newRouterManager(t)
	router := NewActionRouter(manager, &recordingExecutor{}, time.Second)
	for _, target := range []SessionTarget{
		{Session: "work"},
		{HostID: "unknown", Session: "work"},
	} {
		_, err := router.Execute(context.Background(), "rename", target, json.RawMessage(`{}`))
		if err == nil {
			t.Fatalf("target %#v was accepted", target)
		}
	}
}
