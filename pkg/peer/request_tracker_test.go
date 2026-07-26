package peer

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func validRuntimeRequest(id string, generation uint64) RuntimeRequest {
	return RuntimeRequest{
		RequestID: id, Generation: generation, Deadline: time.Now().Add(time.Minute),
		Operation: "rename", Target: SessionTarget{HostID: "peer", Session: "old"},
		Payload: json.RawMessage(`{"new_name":"new"}`),
	}
}

func TestRequestTrackerAckIsNonTerminalAndTerminalIsUnique(t *testing.T) {
	tracker := NewRequestTracker()
	request := validRuntimeRequest("request", 3)
	outcomes, err := tracker.Register(request)
	if err != nil {
		t.Fatal(err)
	}
	if !tracker.Accept(RuntimeAck{RequestID: request.RequestID, Generation: 3, Accepted: true}) {
		t.Fatal("accepted ack was rejected")
	}
	select {
	case <-outcomes:
		t.Fatal("ack incorrectly completed the request")
	default:
	}
	result := RuntimeResult{RequestID: request.RequestID, Generation: 3, Result: json.RawMessage(`{"ok":true}`)}
	if !tracker.CompleteResult(result) {
		t.Fatal("terminal result was not accepted")
	}
	if tracker.CompleteResult(result) {
		t.Fatal("duplicate terminal result was accepted")
	}
	outcome := <-outcomes
	if outcome.Result == nil || outcome.Error != nil {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
}

func TestRequestTrackerGenerationAndFailAll(t *testing.T) {
	tracker := NewRequestTracker()
	request := validRuntimeRequest("request", 7)
	outcomes, _ := tracker.Register(request)
	if tracker.CompleteError(RuntimeError{RequestID: "request", Generation: 6, Code: ErrorPeerOffline}) {
		t.Fatal("old generation completed request")
	}
	tracker.FailAll(7, ErrorPeerRevoked)
	outcome := <-outcomes
	if outcome.Error == nil || outcome.Error.Code != ErrorPeerRevoked {
		t.Fatalf("outcome = %#v", outcome)
	}
}

type fakeRuntimeExecutor struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	err     error
}

func (executor *fakeRuntimeExecutor) Execute(ctx context.Context, _ string, _ SessionTarget, _ json.RawMessage) (json.RawMessage, error) {
	executor.calls.Add(1)
	if executor.entered != nil {
		select {
		case executor.entered <- struct{}{}:
		default:
		}
	}
	if executor.release != nil {
		select {
		case <-executor.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if executor.err != nil {
		return nil, executor.err
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func TestOutcomeCacheMergesInflightAndReplaysCompleted(t *testing.T) {
	cache, _ := NewOutcomeCache(DefaultOutcomeCacheConfig(), nil)
	request := validRuntimeRequest("same", 1)
	executor := &fakeRuntimeExecutor{entered: make(chan struct{}, 1), release: make(chan struct{})}
	dispatcher := &RequestDispatcher{
		Generation: 1, HostID: "peer",
		Capabilities: map[string]struct{}{CapabilitySessionActions: {}},
		Executor:     executor, Cache: cache,
	}
	var outcomes [2]RequestOutcome
	var wait sync.WaitGroup
	for index := range outcomes {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, outcomes[index] = dispatcher.Dispatch(context.Background(), request)
		}(index)
	}
	<-executor.entered
	close(executor.release)
	wait.Wait()
	if executor.calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls.Load())
	}
	_, replay := dispatcher.Dispatch(context.Background(), request)
	if replay.Result == nil || executor.calls.Load() != 1 {
		t.Fatal("completed outcome was not replayed")
	}
}

func TestOutcomeCacheConflictCapacityAndDispatcherValidation(t *testing.T) {
	cache, _ := NewOutcomeCache(OutcomeCacheConfig{TTL: time.Hour, MaxEntries: 1, MaxResultSize: 1024}, nil)
	executor := &fakeRuntimeExecutor{}
	dispatcher := &RequestDispatcher{
		Generation: 1, HostID: "peer",
		Capabilities: map[string]struct{}{CapabilitySessionActions: {}},
		Executor:     executor, Cache: cache,
	}
	first := validRuntimeRequest("one", 1)
	_, _ = dispatcher.Dispatch(context.Background(), first)
	conflict := first
	conflict.Payload = json.RawMessage(`{"new_name":"different"}`)
	_, outcome := dispatcher.Dispatch(context.Background(), conflict)
	if outcome.Error == nil || outcome.Error.Code != ErrorRequestConflict {
		t.Fatalf("conflict outcome = %#v", outcome)
	}
	second := validRuntimeRequest("two", 1)
	_, outcome = dispatcher.Dispatch(context.Background(), second)
	if outcome.Error == nil || outcome.Error.Code != ErrorResourceExhausted {
		t.Fatalf("capacity outcome = %#v", outcome)
	}
	wrongHost := validRuntimeRequest("wrong", 1)
	wrongHost.Target.HostID = "other"
	_, outcome = dispatcher.Dispatch(context.Background(), wrongHost)
	if outcome.Error == nil || outcome.Error.Code != ErrorInvalidTarget {
		t.Fatalf("wrong host outcome = %#v", outcome)
	}
}

func TestPeerRequestReturnsQueueAndTimeoutErrors(t *testing.T) {
	connection := newPeerConnection(context.Background(), "peer", 1, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	filler, _ := NewMessage("filler", nil)
	if err := connection.Send(context.Background(), filler); err != nil {
		t.Fatal(err)
	}
	request := validRuntimeRequest("queue", 1)
	_, err := connection.Request(context.Background(), request)
	var runtimeError RuntimeError
	if !errors.As(err, &runtimeError) || runtimeError.Code != ErrorQueueFull {
		t.Fatalf("request error = %v", err)
	}
}

func TestPeerRequestAckThenTimeoutAndDisconnectAreTerminal(t *testing.T) {
	var timeoutConnection *PeerConnection
	timeoutConnection = newPeerConnection(context.Background(), "peer", 1, RuntimeCapabilities, "instance", 2,
		func(message *Message) error {
			var request RuntimeRequest
			_ = json.Unmarshal(message.Payload, &request)
			timeoutConnection.requests.Accept(RuntimeAck{
				RequestID: request.RequestID, Generation: request.Generation, Accepted: true,
			})
			return nil
		}, nil)
	timeoutConnection.Start()
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := timeoutConnection.Request(timeoutCtx, validRuntimeRequest("ack-timeout", 1))
	var runtimeError RuntimeError
	if !errors.As(err, &runtimeError) || runtimeError.Code != ErrorTimeout {
		t.Fatalf("ack timeout error=%v", err)
	}

	var disconnectConnection *PeerConnection
	entered := make(chan struct{})
	disconnectConnection = newPeerConnection(context.Background(), "peer", 2, RuntimeCapabilities, "instance", 2,
		func(*Message) error {
			close(entered)
			return nil
		}, nil)
	disconnectConnection.Start()
	done := make(chan error, 1)
	request := validRuntimeRequest("disconnect", 2)
	go func() {
		_, err := disconnectConnection.Request(context.Background(), request)
		done <- err
	}()
	<-entered
	disconnectConnection.Close()
	err = <-done
	if !errors.As(err, &runtimeError) || runtimeError.Code != ErrorPeerOffline {
		t.Fatalf("disconnect error=%v", err)
	}
}

func TestOutcomeCacheTTLAndResultSizeBounds(t *testing.T) {
	now := time.Now()
	cache, _ := NewOutcomeCache(OutcomeCacheConfig{
		TTL: time.Second, MaxEntries: 2, MaxResultSize: 160,
	}, func() time.Time { return now })
	executor := &fakeRuntimeExecutor{}
	dispatcher := &RequestDispatcher{
		Generation: 1, HostID: "peer",
		Capabilities: map[string]struct{}{CapabilitySessionActions: {}},
		Executor:     executor, Cache: cache,
	}
	request := validRuntimeRequest("ttl", 1)
	_, _ = dispatcher.Dispatch(context.Background(), request)
	_, _ = dispatcher.Dispatch(context.Background(), request)
	if executor.calls.Load() != 1 {
		t.Fatalf("pre-expiry calls=%d", executor.calls.Load())
	}
	now = now.Add(2 * time.Second)
	_, _ = dispatcher.Dispatch(context.Background(), request)
	if executor.calls.Load() != 2 {
		t.Fatalf("post-expiry calls=%d", executor.calls.Load())
	}

	large := &largeResultExecutor{}
	sizeCache, _ := NewOutcomeCache(OutcomeCacheConfig{
		TTL: time.Minute, MaxEntries: 1, MaxResultSize: 100,
	}, nil)
	sizeDispatcher := &RequestDispatcher{
		Generation: 1, HostID: "peer",
		Capabilities: map[string]struct{}{CapabilitySessionActions: {}},
		Executor:     large, Cache: sizeCache,
	}
	_, outcome := sizeDispatcher.Dispatch(context.Background(), validRuntimeRequest("large", 1))
	if outcome.Error == nil || outcome.Error.Code != ErrorResourceExhausted {
		t.Fatalf("large outcome=%#v", outcome)
	}
	_, replay := sizeDispatcher.Dispatch(context.Background(), validRuntimeRequest("large", 1))
	if replay.Error == nil || large.calls.Load() != 1 {
		t.Fatal("bounded error outcome was not replayed")
	}
}

type largeResultExecutor struct {
	calls atomic.Int32
}

func (executor *largeResultExecutor) Execute(context.Context, string, SessionTarget, json.RawMessage) (json.RawMessage, error) {
	executor.calls.Add(1)
	return json.RawMessage(`{"data":"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"}`), nil
}
