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
			if err := setSessionCookie(rec, NewSessionManager(time.Hour), tt.secure); err != nil {
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
		})
	}
}
