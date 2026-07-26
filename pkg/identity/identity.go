package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

// Identity represents this node's cryptographic identity
type Identity struct {
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`  // base64-encoded ed25519 public key
	PrivateKey string `json:"private_key"` // base64-encoded ed25519 private key
}

// PublicKeyBytes returns the raw ed25519 public key bytes
func (id *Identity) PublicKeyBytes() (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(id.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	return ed25519.PublicKey(b), nil
}

// PrivateKeyBytes returns the raw ed25519 private key bytes
func (id *Identity) PrivateKeyBytes() (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	return ed25519.PrivateKey(b), nil
}

// Sign signs a message with this identity's private key
func (id *Identity) Sign(message []byte) ([]byte, error) {
	priv, err := id.PrivateKeyBytes()
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, message), nil
}

// Fingerprint returns a short identifier derived from the public key
func (id *Identity) Fingerprint() string {
	b, err := base64.StdEncoding.DecodeString(id.PublicKey)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:8])
}

// Generate creates a new identity with a fresh ed25519 keypair
func Generate(name string) (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate keypair: %w", err)
	}

	return &Identity{
		Name:       name,
		PublicKey:  base64.StdEncoding.EncodeToString(pub),
		PrivateKey: base64.StdEncoding.EncodeToString(priv),
	}, nil
}

// Verify checks a signature against a base64-encoded public key
func Verify(publicKeyB64 string, message, signature []byte) bool {
	pub, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), message, signature)
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
			return nil, fmt.Errorf("parse identity: %w", err)
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
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	return &id, nil
}
