package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityStrictValidationAndSigning(t *testing.T) {
	id, err := Generate("node")
	if err != nil {
		t.Fatal(err)
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("generated identity is invalid: %v", err)
	}

	message := []byte("challenge")
	signature, err := id.Sign(message)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(id.PublicKey, message, signature) {
		t.Fatal("valid signature was rejected")
	}
	if Verify(id.PublicKey, message, signature[:len(signature)-1]) {
		t.Fatal("wrong-length signature was accepted")
	}
}

func TestStrictEd25519Parsers(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicEncoded := base64.StdEncoding.EncodeToString(publicKey)
	privateEncoded := base64.StdEncoding.EncodeToString(privateKey)
	signatureEncoded := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, []byte("message")))

	tests := []struct {
		name    string
		parse   func(string) error
		value   string
		wantErr bool
	}{
		{"public valid", func(v string) error { _, err := ParsePublicKey(v); return err }, publicEncoded, false},
		{"public raw base64", func(v string) error { _, err := ParsePublicKey(v); return err }, strings.TrimRight(publicEncoded, "="), true},
		{"public whitespace", func(v string) error { _, err := ParsePublicKey(v); return err }, publicEncoded + "\n", true},
		{"public wrong length", func(v string) error { _, err := ParsePublicKey(v); return err }, base64.StdEncoding.EncodeToString(publicKey[:31]), true},
		{"private valid", func(v string) error { _, err := ParsePrivateKey(v); return err }, privateEncoded, false},
		{"private wrong length", func(v string) error { _, err := ParsePrivateKey(v); return err }, base64.StdEncoding.EncodeToString(privateKey[:32]), true},
		{"signature valid", func(v string) error { _, err := ParseSignature(v); return err }, signatureEncoded, false},
		{"signature wrong length", func(v string) error { _, err := ParseSignature(v); return err }, base64.StdEncoding.EncodeToString(make([]byte, 63)), true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.parse(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestIdentityRejectsMismatchedAndMalformedData(t *testing.T) {
	first, err := Generate("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate("second")
	if err != nil {
		t.Fatal(err)
	}

	first.PrivateKey = second.PrivateKey
	if err := first.Validate(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched keypair error = %v", err)
	}
	if _, err := first.Sign([]byte("message")); err == nil {
		t.Fatal("mismatched identity was allowed to sign")
	}

	first.PublicKey = "not-base64"
	if got := first.Fingerprint(); got != "" {
		t.Fatalf("malformed fingerprint = %q", got)
	}
}

func TestNormalizeName(t *testing.T) {
	if got, err := NormalizeName("  host  "); err != nil || got != "host" {
		t.Fatalf("NormalizeName = %q, %v", got, err)
	}
	for _, value := range []string{"", " \t ", "bad\nname", strings.Repeat("x", MaxPeerNameBytes+1)} {
		if _, err := NormalizeName(value); err == nil {
			t.Fatalf("NormalizeName(%q) unexpectedly succeeded", value)
		}
	}
}

func TestLoadRejectsMalformedIdentityWithoutRewriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "tmuxatlas", "identity.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	id, err := Generate("node")
	if err != nil {
		t.Fatal(err)
	}
	id.PrivateKey = base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize))
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadOrCreate("replacement"); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("LoadOrCreate error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(raw) {
		t.Fatal("malformed identity was rewritten")
	}
}
