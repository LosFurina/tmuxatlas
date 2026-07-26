package peer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type RuntimeExecutor interface {
	Execute(context.Context, string, SessionTarget, json.RawMessage) (json.RawMessage, error)
}

type OutcomeCacheConfig struct {
	TTL           time.Duration
	MaxEntries    int
	MaxResultSize int
}

func DefaultOutcomeCacheConfig() OutcomeCacheConfig {
	return OutcomeCacheConfig{TTL: 5 * time.Minute, MaxEntries: 1024, MaxResultSize: 64 << 10}
}

func OutcomeCacheConfigFromEnv() (OutcomeCacheConfig, error) {
	config := DefaultOutcomeCacheConfig()
	if value := os.Getenv("TMUXATLAS_PEER_OUTCOME_TTL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config, fmt.Errorf("TMUXATLAS_PEER_OUTCOME_TTL: %w", err)
		}
		config.TTL = parsed
	}
	for name, target := range map[string]*int{
		"TMUXATLAS_PEER_OUTCOME_MAX_ENTRIES": &config.MaxEntries,
		"TMUXATLAS_PEER_OUTCOME_MAX_BYTES":   &config.MaxResultSize,
	} {
		if value := os.Getenv(name); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return config, fmt.Errorf("%s: %w", name, err)
			}
			*target = parsed
		}
	}
	if config.TTL <= 0 || config.MaxEntries <= 0 || config.MaxResultSize <= 0 {
		return config, fmt.Errorf("outcome cache limits must be positive")
	}
	return config, nil
}

type cachedOutcome struct {
	digest   [32]byte
	done     chan struct{}
	outcome  RequestOutcome
	expires  time.Time
	complete bool
}

type OutcomeCache struct {
	mu      sync.Mutex
	config  OutcomeCacheConfig
	now     func() time.Time
	entries map[string]*cachedOutcome
}

func NewOutcomeCache(config OutcomeCacheConfig, now func() time.Time) (*OutcomeCache, error) {
	if config.TTL <= 0 || config.MaxEntries <= 0 || config.MaxResultSize <= 0 {
		return nil, fmt.Errorf("outcome cache limits must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &OutcomeCache{config: config, now: now, entries: make(map[string]*cachedOutcome)}, nil
}

func requestDigest(request RuntimeRequest) ([32]byte, error) {
	var payload any
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return [32]byte{}, err
	}
	canonical, err := json.Marshal(struct {
		Operation string        `json:"operation"`
		Target    SessionTarget `json:"target"`
		Payload   any           `json:"payload"`
	}{request.Operation, request.Target, payload})
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(canonical), nil
}

func (cache *OutcomeCache) Do(ctx context.Context, request RuntimeRequest, execute func() RequestOutcome) RequestOutcome {
	digest, err := requestDigest(request)
	if err != nil {
		return errorOutcome(request, ErrorExecutionFailed, "invalid request payload")
	}
	cache.mu.Lock()
	now := cache.now()
	for requestID, entry := range cache.entries {
		if entry.complete && !now.Before(entry.expires) {
			delete(cache.entries, requestID)
		}
	}
	if existing := cache.entries[request.RequestID]; existing != nil {
		if existing.digest != digest {
			cache.mu.Unlock()
			return errorOutcome(request, ErrorRequestConflict, "")
		}
		done := existing.done
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return errorOutcome(request, ErrorTimeout, "")
		case <-done:
			return existing.outcome
		}
	}
	if len(cache.entries) >= cache.config.MaxEntries {
		cache.mu.Unlock()
		return errorOutcome(request, ErrorResourceExhausted, "")
	}
	entry := &cachedOutcome{digest: digest, done: make(chan struct{})}
	cache.entries[request.RequestID] = entry
	cache.mu.Unlock()

	outcome := execute()
	encoded, _ := json.Marshal(outcome)
	if len(encoded) > cache.config.MaxResultSize {
		outcome = errorOutcome(request, ErrorResourceExhausted, "terminal outcome exceeds configured bound")
	}
	cache.mu.Lock()
	entry.outcome = outcome
	entry.complete = true
	entry.expires = cache.now().Add(cache.config.TTL)
	close(entry.done)
	cache.mu.Unlock()
	return outcome
}

type RequestDispatcher struct {
	Generation   uint64
	HostID       string
	Capabilities map[string]struct{}
	Executor     RuntimeExecutor
	Cache        *OutcomeCache
	Now          func() time.Time
}

func (dispatcher *RequestDispatcher) Dispatch(ctx context.Context, request RuntimeRequest) (RuntimeAck, RequestOutcome) {
	now := time.Now
	if dispatcher.Now != nil {
		now = dispatcher.Now
	}
	if err := request.Validate(now()); err != nil {
		return RuntimeAck{}, errorOutcome(request, ErrorInvalidTarget, err.Error())
	}
	if request.Generation != dispatcher.Generation {
		return RuntimeAck{}, errorOutcome(request, ErrorStaleGeneration, "")
	}
	if request.Target.HostID != dispatcher.HostID {
		return RuntimeAck{}, errorOutcome(request, ErrorInvalidTarget, "target host does not match this Agent")
	}
	if _, ok := dispatcher.Capabilities[CapabilitySessionActions]; !ok {
		return RuntimeAck{}, errorOutcome(request, ErrorCapabilityUnsupported, "")
	}
	if dispatcher.Executor == nil || dispatcher.Cache == nil {
		return RuntimeAck{}, errorOutcome(request, ErrorExecutionFailed, "runtime executor unavailable")
	}
	ack := RuntimeAck{RequestID: request.RequestID, Generation: request.Generation, Accepted: true}
	outcome := dispatcher.Cache.Do(ctx, request, func() RequestOutcome {
		result, err := dispatcher.Executor.Execute(ctx, request.Operation, request.Target, request.Payload)
		if err != nil {
			return errorOutcome(request, ErrorExecutionFailed, "target operation failed")
		}
		if len(result) == 0 {
			result = json.RawMessage(`{"ok":true}`)
		}
		terminal := RuntimeResult{RequestID: request.RequestID, Generation: request.Generation, Result: result}
		return RequestOutcome{Result: &terminal}
	})
	return ack, outcome
}

func errorOutcome(request RuntimeRequest, code ErrorCode, message string) RequestOutcome {
	runtimeError := RuntimeError{
		RequestID: request.RequestID, Generation: request.Generation,
		Code: code, Message: message,
	}
	return RequestOutcome{Error: &runtimeError}
}
