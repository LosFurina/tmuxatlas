// Package ingress provides bounded, protocol-neutral admission controls for
// public HTTP and WebSocket entry points.
package ingress

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Category string

const (
	CategoryWebAuthn    Category = "webauthn"
	CategoryBootstrap   Category = "bootstrap"
	CategoryPairing     Category = "pairing"
	CategoryWSTerminal  Category = "ws-terminal"
	CategoryWSPeer      Category = "ws-peer-control"
	CategoryWSPeerPTY   Category = "ws-peer-pty"
	maxSourceLabelBytes          = 128
)

type RejectReason string

const (
	ReasonRateLimited      RejectReason = "rate-limited"
	ReasonConcurrency      RejectReason = "concurrency-limited"
	ReasonSourceCapacity   RejectReason = "source-capacity"
	ReasonUnknownCategory  RejectReason = "unknown-category"
	ReasonInvalidConfig    RejectReason = "invalid-config"
	ReasonRequestTooLarge  RejectReason = "request-too-large"
	ReasonUnsupportedMedia RejectReason = "unsupported-media-type"
	ReasonHostMismatch     RejectReason = "host-mismatch"
	ReasonOriginMismatch   RejectReason = "origin-mismatch"
	ReasonCSRFMismatch     RejectReason = "csrf-mismatch"
)

// Rejection is a stable admission failure safe to map to a public response.
type Rejection struct {
	Category Category
	Reason   RejectReason
}

func (r *Rejection) Error() string {
	if r.Category == "" {
		return string(r.Reason)
	}
	return fmt.Sprintf("%s: %s", r.Category, r.Reason)
}

func IsReason(err error, reason RejectReason) bool {
	var rejection *Rejection
	return errors.As(err, &rejection) && rejection.Reason == reason
}

// HTTPStatus maps stable rejection classes without exposing policy internals.
func HTTPStatus(err error) int {
	var rejection *Rejection
	if !errors.As(err, &rejection) {
		return http.StatusInternalServerError
	}
	switch rejection.Reason {
	case ReasonRateLimited:
		return http.StatusTooManyRequests
	case ReasonConcurrency, ReasonSourceCapacity:
		return http.StatusServiceUnavailable
	case ReasonRequestTooLarge:
		return http.StatusRequestEntityTooLarge
	case ReasonUnsupportedMedia:
		return http.StatusUnsupportedMediaType
	case ReasonHostMismatch, ReasonOriginMismatch, ReasonCSRFMismatch:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

type CategoryLimits struct {
	SourceRate  float64
	SourceBurst int
	GlobalRate  float64
	GlobalBurst int
	MaxInFlight int
}

type Config struct {
	GlobalMaxInFlight     int
	MaxSourcesPerCategory int
	SourceIdleTTL         time.Duration
	CleanupBudget         int
	Categories            map[Category]CategoryLimits
}

// DefaultConfig returns finite conservative limits suitable for a
// single-administrator TmuxAtlas Hub. Callers may override them in tests.
func DefaultConfig() Config {
	return Config{
		GlobalMaxInFlight:     128,
		MaxSourcesPerCategory: 4096,
		SourceIdleTTL:         15 * time.Minute,
		CleanupBudget:         32,
		Categories: map[Category]CategoryLimits{
			CategoryWebAuthn:   {SourceRate: 0.25, SourceBurst: 8, GlobalRate: 4, GlobalBurst: 32, MaxInFlight: 16},
			CategoryBootstrap:  {SourceRate: 0.10, SourceBurst: 4, GlobalRate: 1, GlobalBurst: 16, MaxInFlight: 8},
			CategoryPairing:    {SourceRate: 0.10, SourceBurst: 6, GlobalRate: 2, GlobalBurst: 24, MaxInFlight: 12},
			CategoryWSTerminal: {SourceRate: 0.50, SourceBurst: 8, GlobalRate: 8, GlobalBurst: 64, MaxInFlight: 64},
			CategoryWSPeer:     {SourceRate: 0.25, SourceBurst: 6, GlobalRate: 4, GlobalBurst: 32, MaxInFlight: 64},
			CategoryWSPeerPTY:  {SourceRate: 0.50, SourceBurst: 8, GlobalRate: 8, GlobalBurst: 64, MaxInFlight: 64},
		},
	}
}

func validateConfig(config Config) error {
	if config.GlobalMaxInFlight <= 0 || config.MaxSourcesPerCategory <= 0 ||
		config.SourceIdleTTL <= 0 || config.CleanupBudget <= 0 || len(config.Categories) == 0 {
		return fmt.Errorf("global limits, source capacity, idle TTL, cleanup budget and categories must be finite and positive")
	}
	for category, limits := range config.Categories {
		if category == "" || limits.SourceRate <= 0 || math.IsNaN(limits.SourceRate) || math.IsInf(limits.SourceRate, 0) ||
			limits.GlobalRate <= 0 || math.IsNaN(limits.GlobalRate) || math.IsInf(limits.GlobalRate, 0) ||
			limits.SourceBurst <= 0 || limits.GlobalBurst <= 0 || limits.MaxInFlight <= 0 {
			return fmt.Errorf("category %q has non-finite limits", category)
		}
	}
	return nil
}

type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

func newTokenBucket(burst int, now time.Time) *tokenBucket {
	return &tokenBucket{tokens: float64(burst), last: now, lastSeen: now}
}

func refill(bucket *tokenBucket, rate float64, burst int, now time.Time) {
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens = math.Min(float64(burst), bucket.tokens+elapsed*rate)
		bucket.last = now
	}
	bucket.lastSeen = now
}

type categoryState struct {
	global   *tokenBucket
	sources  map[string]*tokenBucket
	inFlight int
}

// Policy owns rate, concurrency and aggregate rejection state.
type Policy struct {
	mu             sync.Mutex
	config         Config
	now            func() time.Time
	categories     map[Category]*categoryState
	globalInFlight int
	rejections     map[Category]map[RejectReason]uint64
}

func NewPolicy(config Config, now func() time.Time) (*Policy, error) {
	if err := validateConfig(config); err != nil {
		return nil, &Rejection{Reason: ReasonInvalidConfig}
	}
	if now == nil {
		now = time.Now
	}
	current := now()
	policy := &Policy{
		config:     config,
		now:        now,
		categories: make(map[Category]*categoryState, len(config.Categories)),
		rejections: make(map[Category]map[RejectReason]uint64),
	}
	for category, limits := range config.Categories {
		policy.categories[category] = &categoryState{
			global:  newTokenBucket(limits.GlobalBurst, current),
			sources: make(map[string]*tokenBucket),
		}
	}
	return policy, nil
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "unknown"
	}
	if len(source) > maxSourceLabelBytes {
		return source[:maxSourceLabelBytes]
	}
	return source
}

func (p *Policy) reject(category Category, reason RejectReason) error {
	reasons := p.rejections[category]
	if reasons == nil {
		reasons = make(map[RejectReason]uint64)
		p.rejections[category] = reasons
	}
	reasons[reason]++
	return &Rejection{Category: category, Reason: reason}
}

func (p *Policy) cleanupSources(state *categoryState, now time.Time) {
	scanned := 0
	for source, bucket := range state.sources {
		if scanned >= p.config.CleanupBudget {
			return
		}
		scanned++
		if now.Sub(bucket.lastSeen) >= p.config.SourceIdleTTL {
			delete(state.sources, source)
		}
	}
}

// Lease represents one accepted in-flight operation.
type Lease struct {
	once    sync.Once
	release func()
}

func (lease *Lease) Release() {
	if lease != nil {
		lease.once.Do(lease.release)
	}
}

// Acquire charges the per-source and global token buckets and reserves both a
// category and global in-flight slot.
func (p *Policy) Acquire(category Category, source string) (*Lease, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.categories[category]
	if !ok {
		return nil, p.reject(category, ReasonUnknownCategory)
	}
	if p.globalInFlight >= p.config.GlobalMaxInFlight || state.inFlight >= p.config.Categories[category].MaxInFlight {
		return nil, p.reject(category, ReasonConcurrency)
	}

	now := p.now()
	p.cleanupSources(state, now)
	source = normalizeSource(source)
	sourceBucket := state.sources[source]
	if sourceBucket == nil {
		if len(state.sources) >= p.config.MaxSourcesPerCategory {
			return nil, p.reject(category, ReasonSourceCapacity)
		}
		sourceBucket = newTokenBucket(p.config.Categories[category].SourceBurst, now)
		state.sources[source] = sourceBucket
	}

	limits := p.config.Categories[category]
	refill(state.global, limits.GlobalRate, limits.GlobalBurst, now)
	refill(sourceBucket, limits.SourceRate, limits.SourceBurst, now)
	if state.global.tokens < 1 || sourceBucket.tokens < 1 {
		return nil, p.reject(category, ReasonRateLimited)
	}
	state.global.tokens--
	sourceBucket.tokens--
	state.inFlight++
	p.globalInFlight++

	return &Lease{release: func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if state.inFlight > 0 {
			state.inFlight--
		}
		if p.globalInFlight > 0 {
			p.globalInFlight--
		}
	}}, nil
}

// RejectionSnapshot returns aggregate category/reason counters only. It never
// includes source labels or request material.
func (p *Policy) RejectionSnapshot() map[Category]map[RejectReason]uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make(map[Category]map[RejectReason]uint64, len(p.rejections))
	for category, reasons := range p.rejections {
		copyReasons := make(map[RejectReason]uint64, len(reasons))
		for reason, count := range reasons {
			copyReasons[reason] = count
		}
		result[category] = copyReasons
	}
	return result
}
