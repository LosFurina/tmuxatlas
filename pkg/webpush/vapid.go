package webpush

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

// VAPIDKeys holds the public/private VAPID key pair
type VAPIDKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// LoadOrCreateKeys loads VAPID keys from disk, or generates and persists new ones
func LoadOrCreateKeys() (*VAPIDKeys, error) {
	dir, err := paths.DataDir()
	if err != nil {
		return nil, err
	}
	keyFile := filepath.Join(dir, "vapid-keys.json")

	// Try loading existing keys
	data, err := os.ReadFile(keyFile)
	if err == nil {
		var keys VAPIDKeys
		if err := json.Unmarshal(data, &keys); err == nil && keys.PublicKey != "" && keys.PrivateKey != "" {
			return &keys, nil
		}
	}

	// Generate new keys
	priv, pub, err := wp.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("generate VAPID keys: %w", err)
	}

	keys := &VAPIDKeys{
		PublicKey:  pub,
		PrivateKey: priv,
	}

	// Persist to disk
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	data, err = json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal VAPID keys: %w", err)
	}

	if err := os.WriteFile(keyFile, data, 0600); err != nil {
		return nil, fmt.Errorf("write VAPID keys: %w", err)
	}

	return keys, nil
}
