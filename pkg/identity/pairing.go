package identity

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	PairingCodeTTL    = 5 * time.Minute
	MaxPendingPairing = 32
	PairingVersion    = 1
)

var pairingSyllables = [...]string{
	"ba", "be", "bi", "bo", "da", "de", "di", "do",
	"ka", "ke", "ki", "ko", "la", "le", "li", "lo",
}

type PairingCode struct {
	Code      string
	ExpiresAt time.Time
}

type PairingManager struct {
	mu    sync.Mutex
	codes map[string]*PairingCode
	rand  io.Reader
	now   func() time.Time
	max   int
}

func NewPairingManager() *PairingManager {
	return newPairingManager(rand.Reader, time.Now, MaxPendingPairing)
}

func newPairingManager(random io.Reader, now func() time.Time, maxPending int) *PairingManager {
	return &PairingManager{codes: make(map[string]*PairingCode), rand: random, now: now, max: maxPending}
}

// Generate samples six independent bytes. Each byte maps one-to-one onto a
// 256-entry generated word vocabulary, providing exactly 48 bits without
// modulo bias.
func (pm *PairingManager) Generate() (*PairingCode, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	now := pm.now()
	pm.cleanupLocked(now)
	if len(pm.codes) >= pm.max {
		return nil, fmt.Errorf("pending pairing code capacity reached")
	}
	random := make([]byte, 6)
	if _, err := io.ReadFull(pm.rand, random); err != nil {
		return nil, fmt.Errorf("generate random: %w", err)
	}
	parts := make([]string, len(random))
	for i, value := range random {
		parts[i] = pairingSyllables[value>>4] + pairingSyllables[value&0x0f]
	}
	code := strings.ToUpper(strings.Join(parts, "-"))
	pc := &PairingCode{Code: code, ExpiresAt: now.Add(PairingCodeTTL)}
	pm.codes[code] = pc
	return pc, nil
}

// Complete runs commit while holding the code lock and consumes the one-time
// code only after the entire persistence operation succeeds.
func (pm *PairingManager) Complete(code string, commit func() error) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	code = NormalizePairingCode(code)
	pc, ok := pm.codes[code]
	if !ok || !pm.now().Before(pc.ExpiresAt) {
		if ok {
			delete(pm.codes, code)
		}
		return fmt.Errorf("pairing failed")
	}
	if err := commit(); err != nil {
		return fmt.Errorf("pairing failed")
	}
	delete(pm.codes, code)
	return nil
}

func (pm *PairingManager) cleanupLocked(now time.Time) {
	for code, pc := range pm.codes {
		if !now.Before(pc.ExpiresAt) {
			delete(pm.codes, code)
		}
	}
}

func NormalizePairingCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// PairingTranscript creates an unambiguous versioned proof payload.
func PairingTranscript(origin, code, name, publicKey string) []byte {
	var transcript bytes.Buffer
	transcript.WriteString("tmuxatlas/pairing-proof\x00")
	_ = binary.Write(&transcript, binary.BigEndian, uint16(PairingVersion))
	for _, value := range []string{origin, NormalizePairingCode(code), name, publicKey} {
		_ = binary.Write(&transcript, binary.BigEndian, uint32(len(value)))
		transcript.WriteString(value)
	}
	return transcript.Bytes()
}
