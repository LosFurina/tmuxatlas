package identity

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPairingCodeUsesSixUnbiasedByteSelections(t *testing.T) {
	now := time.Unix(100, 0)
	manager := newPairingManager(bytes.NewReader([]byte{0, 1, 15, 16, 254, 255}), func() time.Time {
		return now
	}, 1)
	code, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(code.Code, "-")
	if len(parts) != 6 {
		t.Fatalf("pairing words = %d, want 6", len(parts))
	}
	if code.ExpiresAt.Sub(now) != PairingCodeTTL {
		t.Fatalf("TTL = %s, want %s", code.ExpiresAt.Sub(now), PairingCodeTTL)
	}
	seen := make(map[string]bool)
	for high := 0; high < 16; high++ {
		for low := 0; low < 16; low++ {
			word := pairingSyllables[high] + pairingSyllables[low]
			if seen[word] {
				t.Fatalf("duplicate vocabulary word %q", word)
			}
			seen[word] = true
		}
	}
	if len(seen) != 256 {
		t.Fatalf("vocabulary size = %d, want 256", len(seen))
	}
}

func TestPairingCompleteRetainsCodeAfterFailedCommit(t *testing.T) {
	manager := newPairingManager(bytes.NewReader(make([]byte, 12)), time.Now, 2)
	code, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Complete(code.Code, func() error { return errors.New("disk full") }); err == nil {
		t.Fatal("failed commit succeeded")
	}
	if err := manager.Complete(code.Code, func() error { return nil }); err != nil {
		t.Fatalf("code was consumed by failed commit: %v", err)
	}
}

func TestPairingCompleteAllowsOneConcurrentSuccess(t *testing.T) {
	manager := newPairingManager(bytes.NewReader(make([]byte, 6)), time.Now, 1)
	code, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if manager.Complete(code.Code, func() error { return nil }) == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes = %d, want 1", successes.Load())
	}
}

func TestPairingTranscriptIsLengthDelimited(t *testing.T) {
	first := PairingTranscript("https://hub", "A-B", "C", "key")
	second := PairingTranscript("https://hub", "A", "B-C", "key")
	if bytes.Equal(first, second) {
		t.Fatal("different fields produced the same transcript")
	}
}
