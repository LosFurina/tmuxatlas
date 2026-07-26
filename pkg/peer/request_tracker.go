package peer

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type RequestOutcome struct {
	Result *RuntimeResult
	Error  *RuntimeError
}

type pendingRequest struct {
	generation uint64
	outcome    chan RequestOutcome
	once       sync.Once
}

type RequestTracker struct {
	mu      sync.Mutex
	pending map[string]*pendingRequest
}

func NewRequestTracker() *RequestTracker {
	return &RequestTracker{pending: make(map[string]*pendingRequest)}
}

func (tracker *RequestTracker) Register(request RuntimeRequest) (<-chan RequestOutcome, error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.pending[request.RequestID]; exists {
		return nil, RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: ErrorRequestConflict}
	}
	pending := &pendingRequest{generation: request.Generation, outcome: make(chan RequestOutcome, 1)}
	tracker.pending[request.RequestID] = pending
	return pending.outcome, nil
}

// Accept records a non-terminal acknowledgement. It deliberately does not
// remove or complete the pending request.
func (tracker *RequestTracker) Accept(ack RuntimeAck) bool {
	if ack.Validate() != nil {
		return false
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	pending := tracker.pending[ack.RequestID]
	return pending != nil && pending.generation == ack.Generation
}

func (tracker *RequestTracker) CompleteResult(result RuntimeResult) bool {
	if result.Validate() != nil {
		return false
	}
	return tracker.complete(result.RequestID, result.Generation, RequestOutcome{Result: &result})
}

func (tracker *RequestTracker) CompleteError(runtimeError RuntimeError) bool {
	if runtimeError.ValidateCorrelated() != nil {
		return false
	}
	return tracker.complete(runtimeError.RequestID, runtimeError.Generation, RequestOutcome{Error: &runtimeError})
}

func (tracker *RequestTracker) complete(requestID string, generation uint64, outcome RequestOutcome) bool {
	tracker.mu.Lock()
	pending := tracker.pending[requestID]
	if pending == nil || pending.generation != generation {
		tracker.mu.Unlock()
		return false
	}
	delete(tracker.pending, requestID)
	tracker.mu.Unlock()
	completed := false
	pending.once.Do(func() {
		pending.outcome <- outcome
		close(pending.outcome)
		completed = true
	})
	return completed
}

func (tracker *RequestTracker) FailAll(generation uint64, code ErrorCode) {
	tracker.mu.Lock()
	selected := make(map[string]*pendingRequest)
	for requestID, pending := range tracker.pending {
		if pending.generation == generation {
			selected[requestID] = pending
			delete(tracker.pending, requestID)
		}
	}
	tracker.mu.Unlock()
	for requestID, pending := range selected {
		requestID, pending := requestID, pending
		pending.once.Do(func() {
			runtimeError := RuntimeError{RequestID: requestID, Generation: generation, Code: code}
			pending.outcome <- RequestOutcome{Error: &runtimeError}
			close(pending.outcome)
		})
	}
}

func (connection *PeerConnection) Request(ctx context.Context, request RuntimeRequest) (json.RawMessage, error) {
	if request.Generation != connection.Generation {
		return nil, RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: ErrorStaleGeneration}
	}
	if err := request.Validate(time.Now()); err != nil {
		return nil, RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: ErrorInvalidTarget, Message: err.Error()}
	}
	outcomes, err := connection.requests.Register(request)
	if err != nil {
		return nil, err
	}
	message, err := NewMessage(MsgRuntimeRequest, request)
	if err != nil {
		connection.requests.CompleteError(RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: ErrorExecutionFailed})
		return nil, err
	}
	if err := connection.Send(ctx, message); err != nil {
		code := ErrorPeerOffline
		if errors.Is(err, ErrQueueFull) {
			code = ErrorQueueFull
		}
		connection.requests.CompleteError(RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: code})
	}
	select {
	case outcome := <-outcomes:
		if outcome.Error != nil {
			return nil, *outcome.Error
		}
		return outcome.Result.Result, nil
	case <-ctx.Done():
		timeoutError := RuntimeError{RequestID: request.RequestID, Generation: request.Generation, Code: ErrorTimeout}
		if connection.requests.CompleteError(timeoutError) {
			return nil, timeoutError
		}
		outcome := <-outcomes
		if outcome.Error != nil {
			return nil, *outcome.Error
		}
		return outcome.Result.Result, nil
	}
}
