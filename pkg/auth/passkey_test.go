package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func newTestPasskeyStore(t *testing.T) *PasskeyStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "passkeys.json")
	store := &PasskeyStore{
		path: path,
		data: passkeyData{
			Version: 1,
			RPID:    "tmuxatlas.example.com",
			Origin:  "https://tmuxatlas.example.com",
			UserID:  []byte("stable-user-handle"),
		},
	}
	if err := store.saveLocked(); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestBootstrapTokenIsRequiredAndConsumed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager, err := NewPasskeyManager(
		"http://localhost:7654",
		NewSessionManager(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	token := manager.BootstrapToken()
	if token == "" {
		t.Fatal("expected bootstrap token")
	}

	call := func(supplied string) *httptest.ResponseRecorder {
		body := `{"setup_token":"` + supplied + `","label":"test"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/passkey/register/begin", strings.NewReader(body))
		rec := httptest.NewRecorder()
		manager.BeginRegistrationHandler().ServeHTTP(rec, req)
		return rec
	}
	if got := call("wrong").Code; got != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want 401", got)
	}
	first := call(token)
	if first.Code != http.StatusOK {
		t.Fatalf("valid token status = %d, body = %s", first.Code, first.Body.String())
	}
	if len(first.Result().Cookies()) != 1 || first.Result().Cookies()[0].Name != ceremonyCookieName {
		t.Fatal("registration ceremony cookie was not set")
	}
	if got := call(token).Code; got != http.StatusUnauthorized {
		t.Fatalf("reused token status = %d, want 401", got)
	}
}

func TestPasskeyStorePersistsCredentialAndProtectsFile(t *testing.T) {
	store := newTestPasskeyStore(t)
	credential := &webauthn.Credential{ID: []byte("credential-id"), PublicKey: []byte("public-key")}
	if err := store.add("iPhone", credential); err != nil {
		t.Fatal(err)
	}
	if !store.HasCredentials() {
		t.Fatal("expected stored credential")
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("passkey store mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("password_hash")) {
		t.Fatal("legacy password material must not be written to passkey store")
	}
}

func TestPasskeyStoreRejectsDuplicateAndUpdatesCounterRecord(t *testing.T) {
	store := newTestPasskeyStore(t)
	original := &webauthn.Credential{ID: []byte("credential-id"), PublicKey: []byte("public-key")}
	if err := store.add("Bitwarden", original); err != nil {
		t.Fatal(err)
	}
	if err := store.add("duplicate", original); err == nil {
		t.Fatal("expected duplicate credential to be rejected")
	}

	updated := *original
	updated.Authenticator.SignCount = 42
	if err := store.update(&updated); err != nil {
		t.Fatal(err)
	}
	if got := store.snapshot().credentials[0].Authenticator.SignCount; got != 42 {
		t.Fatalf("sign count = %d, want 42", got)
	}
}
