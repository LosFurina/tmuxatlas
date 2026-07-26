package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionCookieSecurityFollowsPublicURLPolicy(t *testing.T) {
	for _, tt := range []struct {
		name   string
		secure bool
	}{
		{name: "local HTTP", secure: false},
		{name: "gateway HTTPS", secure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ttl := 2 * time.Hour
			if err := setSessionCookie(rec, NewSessionManager(ttl), tt.secure); err != nil {
				t.Fatal(err)
			}
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies = %d", len(cookies))
			}
			if cookies[0].Secure != tt.secure {
				t.Errorf("Secure = %v, want %v", cookies[0].Secure, tt.secure)
			}
			if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie security attributes not preserved: %#v", cookies[0])
			}
			if cookies[0].MaxAge != int(ttl/time.Second) {
				t.Errorf("MaxAge = %d, want %d", cookies[0].MaxAge, int(ttl/time.Second))
			}
		})
	}
}

func TestAuthenticatedRequestRefreshesSessionCookie(t *testing.T) {
	sm := NewSessionManager(90 * time.Minute)
	token, err := sm.Create()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	rec := httptest.NewRecorder()

	Middleware(sm, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].Value != token || cookies[0].MaxAge != 5400 || !cookies[0].Secure {
		t.Fatalf("session cookie was not refreshed correctly: %#v", cookies[0])
	}
}
