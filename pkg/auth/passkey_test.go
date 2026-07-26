package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
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
	var registrationOptions struct {
		PublicKey struct {
			AuthenticatorSelection struct {
				ResidentKey        string `json:"residentKey"`
				RequireResidentKey bool   `json:"requireResidentKey"`
				UserVerification   string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	if err := json.NewDecoder(first.Body).Decode(&registrationOptions); err != nil {
		t.Fatal(err)
	}
	selection := registrationOptions.PublicKey.AuthenticatorSelection
	if selection.ResidentKey != "required" || !selection.RequireResidentKey || selection.UserVerification != "required" {
		t.Fatalf("registration authenticator selection = %+v, want resident and user verification required", selection)
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

func TestPasskeyMetadataRenameAndDelete(t *testing.T) {
	store := newTestPasskeyStore(t)
	first := &webauthn.Credential{ID: []byte("first"), PublicKey: []byte("public-one")}
	second := &webauthn.Credential{ID: []byte("second"), PublicKey: []byte("public-two")}
	if err := store.add(" iPhone ", first); err != nil {
		t.Fatal(err)
	}
	if err := store.add("Bitwarden", second); err != nil {
		t.Fatal(err)
	}
	items := store.List()
	if len(items) != 2 || items[0].Label != "iPhone" {
		t.Fatalf("metadata = %#v", items)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("public-one")) || bytes.Contains(raw, []byte("publicKey")) {
		t.Fatalf("metadata leaked credential material: %s", raw)
	}
	if err := store.Rename(items[0].ID, "  MacBook Touch ID  "); err != nil {
		t.Fatal(err)
	}
	if got := store.List()[0].Label; got != "MacBook Touch ID" {
		t.Fatalf("renamed label = %q", got)
	}
	if err := store.Rename(items[0].ID, strings.Repeat("界", 81)); !errors.Is(err, ErrInvalidPasskeyLabel) {
		t.Fatalf("over-length rename error = %v", err)
	}
	if err := store.Delete(items[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(store.List()[0].ID); !errors.Is(err, ErrLastPasskey) {
		t.Fatalf("final delete error = %v", err)
	}
	if got := store.List(); len(got) != 1 || got[0].Label != "Bitwarden" {
		t.Fatalf("remaining metadata = %#v", got)
	}
}

func TestConcurrentDeleteAlwaysLeavesCredential(t *testing.T) {
	store := newTestPasskeyStore(t)
	for _, id := range []string{"one", "two"} {
		if err := store.add(id, &webauthn.Credential{ID: []byte(id), PublicKey: []byte("public-" + id)}); err != nil {
			t.Fatal(err)
		}
	}
	items := store.List()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, item := range items {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			results <- store.Delete(id)
		}(item.ID)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrLastPasskey):
			conflicts++
		default:
			t.Fatalf("unexpected delete error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 || len(store.List()) != 1 {
		t.Fatalf("successes=%d conflicts=%d remaining=%d", successes, conflicts, len(store.List()))
	}
}

func TestPasskeyManagementRoutesRequireSession(t *testing.T) {
	store := newTestPasskeyStore(t)
	if err := store.add("test", &webauthn.Credential{ID: []byte("credential"), PublicKey: []byte("public")}); err != nil {
		t.Fatal(err)
	}
	sessions := NewSessionManager(time.Hour)
	manager := &PasskeyManager{store: store, sessions: sessions}
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(Middleware(sessions, false))
		r.Get("/api/auth/passkeys", manager.ListHandler())
		r.Patch("/api/auth/passkeys/{credentialID}", manager.RenameHandler())
		r.Delete("/api/auth/passkeys/{credentialID}", manager.DeleteHandler())
	})

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/auth/passkeys", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	token, err := sessions.Create()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/auth/passkeys", nil)
	request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", authorized.Code, authorized.Body.String())
	}
	if strings.Contains(authorized.Body.String(), "public") {
		t.Fatalf("handler leaked credential material: %s", authorized.Body.String())
	}
}
