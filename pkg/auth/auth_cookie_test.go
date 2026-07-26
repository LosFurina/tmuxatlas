package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupCookieSecurityFollowsPublicURLPolicy(t *testing.T) {
	for _, tt := range []struct {
		name   string
		secure bool
	}{
		{name: "local HTTP", secure: false},
		{name: "gateway HTTPS", secure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ps := &PasswordStore{path: filepath.Join(t.TempDir(), "auth.json")}
			sm := NewSessionManager(time.Hour)
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"password":"long-enough-password"}`))
			rec := httptest.NewRecorder()

			SetupHandler(ps, sm, tt.secure).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
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
		})
	}
}
