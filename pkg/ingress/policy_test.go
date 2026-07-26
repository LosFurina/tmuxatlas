package ingress

import (
	"net/http"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (clock *fakeClock) Now() time.Time {
	return clock.now
}

func testConfig() Config {
	return Config{
		GlobalMaxInFlight:     2,
		MaxSourcesPerCategory: 2,
		SourceIdleTTL:         time.Minute,
		CleanupBudget:         2,
		Categories: map[Category]CategoryLimits{
			CategoryPairing:  {SourceRate: 1, SourceBurst: 2, GlobalRate: 2, GlobalBurst: 3, MaxInFlight: 2},
			CategoryWebAuthn: {SourceRate: 1, SourceBurst: 2, GlobalRate: 2, GlobalBurst: 3, MaxInFlight: 2},
		},
	}
}

func TestPolicyRateLimitAndRefill(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	policy, err := NewPolicy(testConfig(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		lease, err := policy.Acquire(CategoryPairing, "192.0.2.1")
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		lease.Release()
	}
	if _, err := policy.Acquire(CategoryPairing, "192.0.2.1"); !IsReason(err, ReasonRateLimited) {
		t.Fatalf("rate limit error = %v", err)
	}
	clock.now = clock.now.Add(time.Second)
	lease, err := policy.Acquire(CategoryPairing, "192.0.2.1")
	if err != nil {
		t.Fatalf("Acquire after refill: %v", err)
	}
	lease.Release()

	snapshot := policy.RejectionSnapshot()
	if snapshot[CategoryPairing][ReasonRateLimited] != 1 {
		t.Fatalf("rejection metrics = %#v", snapshot)
	}
}

func TestPolicyGlobalRateAcrossSources(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	config := testConfig()
	config.Categories[CategoryPairing] = CategoryLimits{
		SourceRate: 10, SourceBurst: 10, GlobalRate: 1, GlobalBurst: 2, MaxInFlight: 2,
	}
	policy, err := NewPolicy(config, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"one", "two"} {
		lease, err := policy.Acquire(CategoryPairing, source)
		if err != nil {
			t.Fatal(err)
		}
		lease.Release()
	}
	if _, err := policy.Acquire(CategoryPairing, "one"); !IsReason(err, ReasonRateLimited) {
		t.Fatalf("global rate error = %v", err)
	}
}

func TestPolicyConcurrencyAndCategoryIsolation(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	policy, err := NewPolicy(testConfig(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := policy.Acquire(CategoryPairing, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := policy.Acquire(CategoryPairing, "two")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Acquire(CategoryWebAuthn, "one"); !IsReason(err, ReasonConcurrency) {
		t.Fatalf("global concurrency error = %v", err)
	}
	first.Release()
	first.Release()
	other, err := policy.Acquire(CategoryWebAuthn, "one")
	if err != nil {
		t.Fatalf("independent category did not recover: %v", err)
	}
	other.Release()
	second.Release()
}

func TestPolicySourceCapacityAndBoundedCleanup(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1, 0)}
	policy, err := NewPolicy(testConfig(), clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"one", "two"} {
		lease, err := policy.Acquire(CategoryPairing, source)
		if err != nil {
			t.Fatal(err)
		}
		lease.Release()
	}
	if _, err := policy.Acquire(CategoryPairing, "three"); !IsReason(err, ReasonSourceCapacity) {
		t.Fatalf("source capacity error = %v", err)
	}
	clock.now = clock.now.Add(time.Minute)
	lease, err := policy.Acquire(CategoryPairing, "three")
	if err != nil {
		t.Fatalf("expired source entries were not cleaned: %v", err)
	}
	lease.Release()
}

func TestPolicyStableHTTPStatusAndDefaults(t *testing.T) {
	if _, err := NewPolicy(DefaultConfig(), nil); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
	tests := []struct {
		reason RejectReason
		status int
	}{
		{ReasonRateLimited, http.StatusTooManyRequests},
		{ReasonConcurrency, http.StatusServiceUnavailable},
		{ReasonSourceCapacity, http.StatusServiceUnavailable},
		{ReasonRequestTooLarge, http.StatusRequestEntityTooLarge},
		{ReasonUnsupportedMedia, http.StatusUnsupportedMediaType},
		{ReasonHostMismatch, http.StatusForbidden},
		{ReasonOriginMismatch, http.StatusForbidden},
		{ReasonCSRFMismatch, http.StatusForbidden},
	}
	for _, test := range tests {
		if got := HTTPStatus(&Rejection{Reason: test.reason}); got != test.status {
			t.Errorf("HTTPStatus(%s) = %d, want %d", test.reason, got, test.status)
		}
	}
}

func TestPolicyRejectsUnboundedConfiguration(t *testing.T) {
	config := testConfig()
	config.GlobalMaxInFlight = 0
	if _, err := NewPolicy(config, nil); !IsReason(err, ReasonInvalidConfig) {
		t.Fatalf("invalid config error = %v", err)
	}
}
