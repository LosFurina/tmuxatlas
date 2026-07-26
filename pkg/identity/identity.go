package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

const MaxPeerNameBytes = 128

// Identity represents this node's cryptographic identity
type Identity struct {
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`  // base64-encoded ed25519 public key
	PrivateKey string `json:"private_key"` // base64-encoded ed25519 private key
}

// NormalizeName validates and returns the canonical peer display name.
func NormalizeName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("name is empty")
	}
	if !utf8.ValidString(normalized) {
		return "", fmt.Errorf("name is not valid UTF-8")
	}
	if len(normalized) > MaxPeerNameBytes {
		return "", fmt.Errorf("name exceeds %d bytes", MaxPeerNameBytes)
	}
	for _, r := range normalized {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("name contains control characters")
		}
	}
	return normalized, nil
}

func validateStoredName(name string) error {
	normalized, err := NormalizeName(name)
	if err != nil {
		return err
	}
	if normalized != name {
		return fmt.Errorf("name is not canonical (remove surrounding whitespace)")
	}
	return nil
}

func decodeCanonicalBase64(field, encoded string, size int) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s is not canonical padded Base64: %w", field, err)
	}
	if base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, fmt.Errorf("%s is not canonical padded Base64", field)
	}
	if len(decoded) != size {
		return nil, fmt.Errorf("%s has %d bytes, want %d", field, len(decoded), size)
	}
	return decoded, nil
}

// ParsePublicKey validates and decodes a canonical Ed25519 public key.
func ParsePublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := decodeCanonicalBase64("public key", encoded, ed25519.PublicKeySize)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(decoded), nil
}

// ParsePrivateKey validates and decodes a canonical Ed25519 private key.
func ParsePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	decoded, err := decodeCanonicalBase64("private key", encoded, ed25519.PrivateKeySize)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(decoded), nil
}

// ParseSignature validates and decodes a canonical Ed25519 signature.
func ParseSignature(encoded string) ([]byte, error) {
	return decodeCanonicalBase64("signature", encoded, ed25519.SignatureSize)
}

// PublicKeyBytes returns the raw ed25519 public key bytes
func (id *Identity) PublicKeyBytes() (ed25519.PublicKey, error) {
	return ParsePublicKey(id.PublicKey)
}

// PrivateKeyBytes returns the raw ed25519 private key bytes
func (id *Identity) PrivateKeyBytes() (ed25519.PrivateKey, error) {
	return ParsePrivateKey(id.PrivateKey)
}

// Validate verifies the complete local identity before cryptographic use.
func (id *Identity) Validate() error {
	if id == nil {
		return fmt.Errorf("identity is nil")
	}
	if err := validateStoredName(id.Name); err != nil {
		return fmt.Errorf("invalid identity name: %w", err)
	}
	publicKey, err := id.PublicKeyBytes()
	if err != nil {
		return err
	}
	privateKey, err := id.PrivateKeyBytes()
	if err != nil {
		return err
	}
	derived, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(publicKey, derived) {
		return fmt.Errorf("public key does not match private key")
	}
	return nil
}

// Sign signs a message with this identity's private key
func (id *Identity) Sign(message []byte) ([]byte, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("validate identity: %w", err)
	}
	priv, err := id.PrivateKeyBytes()
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, message), nil
}

// Fingerprint returns a short identifier derived from the public key
func (id *Identity) Fingerprint() string {
	b, err := id.PublicKeyBytes()
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:8])
}

// Generate creates a new identity with a fresh ed25519 keypair
func Generate(name string) (*Identity, error) {
	normalizedName, err := NormalizeName(name)
	if err != nil {
		return nil, fmt.Errorf("invalid identity name: %w", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	return &Identity{
		Name:       normalizedName,
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: base64.StdEncoding.EncodeToString(priv),
	}, nil
}

// Verify checks a signature against a base64-encoded public key
func Verify(publicKeyB64 string, message, signature []byte) bool {
	pub, err := ParsePublicKey(publicKeyB64)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, message, signature)
}

// configDir returns the TmuxAtlas config directory, creating it if needed.
func configDir() (string, error) {
	return paths.ConfigDir()
}

// identityPath returns the path to the identity file
func identityPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.json"), nil
}

// LoadOrCreate loads the identity from disk, or generates a new one
func LoadOrCreate(defaultName string) (*Identity, error) {
	path, err := identityPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var id Identity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, fmt.Errorf("parse identity %s: %w", path, err)
		}
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("validate identity %s: %w", path, err)
		}
		return &id, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read identity: %w", err)
	}

	// Generate new identity
	id, err := Generate(defaultName)
	if err != nil {
		return nil, err
	}

	data, err = json.MarshalIndent(id, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal identity: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, fmt.Errorf("write identity: %w", err)
	}

	return id, nil
}

// Load loads the identity from disk, returning an error if it doesn't exist
func Load() (*Identity, error) {
	path, err := identityPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read identity: %w", err)
	}

	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parse identity %s: %w", path, err)
	}
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("validate identity %s: %w", path, err)
	}
	return &id, nil
}

// ValidateStoredIdentity inspects the persisted local identity without
// creating or rewriting it. It is intended for diagnostics.
func ValidateStoredIdentity() error {
	_, err := Load()
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	return err
}
