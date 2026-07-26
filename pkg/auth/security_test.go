package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestSessionCSRFIsBoundToSession(t *testing.T) {
	manager := NewSessionManager(time.Hour)
	first, err := manager.Create()
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create()
	if err != nil {
		t.Fatal(err)
	}
	firstCSRF, ok := manager.CSRF(first)
	if !ok || firstCSRF == "" {
		t.Fatal("first session has no CSRF token")
	}
	secondCSRF, ok := manager.CSRF(second)
	if !ok || secondCSRF == "" || firstCSRF == secondCSRF {
		t.Fatal("CSRF tokens are missing or shared across sessions")
	}
}

func TestBootstrapExpiryAndRotationLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager, err := NewPasskeyManager("http://localhost:7654", NewSessionManager(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	initial := manager.BootstrapToken()
	manager.mu.Lock()
	manager.bootstrapExpires = time.Now().Add(-time.Second)
	manager.mu.Unlock()
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	if manager.authorizeRegistration(request, initial) {
		t.Fatal("expired bootstrap token was accepted")
	}
	rotated, err := manager.RotateBootstrapToken()
	if err != nil || rotated == "" || rotated == initial {
		t.Fatalf("rotation failed: token=%q err=%v", rotated, err)
	}
	if err := manager.store.add("primary", &webauthn.Credential{ID: []byte("credential")}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RotateBootstrapToken(); err == nil {
		t.Fatal("rotation succeeded after credential enrollment")
	}
}

func TestOriginAndCSRFMiddleware(t *testing.T) {
	manager := NewSessionManager(time.Hour)
	token, err := manager.Create()
	if err != nil {
		t.Fatal(err)
	}
	csrf, _ := manager.CSRF(token)
	origin, err := OriginMiddleware("https://TmuxAtlas.Example")
	if err != nil {
		t.Fatal(err)
	}
	handler := origin(CSRFMiddleware(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	tests := []struct {
		name       string
		origin     string
		csrf       string
		wantStatus int
	}{
		{name: "valid default port", origin: "https://tmuxatlas.example:443", csrf: csrf, wantStatus: http.StatusNoContent},
		{name: "same site different origin", origin: "https://admin.tmuxatlas.example", csrf: csrf, wantStatus: http.StatusForbidden},
		{name: "missing origin", csrf: csrf, wantStatus: http.StatusForbidden},
		{name: "wrong csrf", origin: "https://tmuxatlas.example", csrf: "wrong", wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Header.Set(CSRFHeader, tt.csrf)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
