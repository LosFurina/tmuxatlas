package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SessionManager manages in-memory session tokens with expiry.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]session
	ttl      time.Duration
}

type session struct {
	expiry time.Time
	csrf   string
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{sessions: make(map[string]session), ttl: ttl}
}

func (sm *SessionManager) TTL() time.Duration {
	return sm.ttl
}

func (sm *SessionManager) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		return "", err
	}
	sm.mu.Lock()
	sm.sessions[token] = session{expiry: time.Now().Add(sm.ttl), csrf: hex.EncodeToString(csrfBytes)}
	sm.mu.Unlock()
	return token, nil
}

func (sm *SessionManager) Validate(token string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	value, ok := sm.sessions[token]
	if !ok || time.Now().After(value.expiry) {
		delete(sm.sessions, token)
		return false
	}
	value.expiry = time.Now().Add(sm.ttl)
	sm.sessions[token] = value
	return true
}

func (sm *SessionManager) CSRF(token string) (string, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	value, ok := sm.sessions[token]
	if !ok || time.Now().After(value.expiry) {
		return "", false
	}
	return value.csrf, true
}

func (sm *SessionManager) Revoke(token string) {
	sm.mu.Lock()
	delete(sm.sessions, token)
	sm.mu.Unlock()
}

func (sm *SessionManager) Cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	now := time.Now()
	for token, value := range sm.sessions {
		if now.After(value.expiry) {
			delete(sm.sessions, token)
		}
	}
}

const cookieName = "tmuxatlas_session"

func isUnixSocket(r *http.Request) bool {
	addr := r.Context().Value(http.LocalAddrContextKey)
	if addr == nil {
		return false
	}
	_, ok := addr.(*net.UnixAddr)
	return ok
}

// Middleware enforces a browser session. Local CLI requests over the Unix
// socket remain trusted.
func Middleware(sm *SessionManager, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isUnixSocket(r) {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil || !sm.Validate(cookie.Value) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			refreshSessionCookie(w, cookie.Value, sm.TTL(), secureCookies)
			next.ServeHTTP(w, r)
		})
	}
}

const CSRFHeader = "X-TmuxAtlas-CSRF"

// CSRFMiddleware protects cookie-authenticated browser mutations. It must run
// after Middleware so that the active session has already been validated.
func CSRFMiddleware(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				writeError(w, http.StatusForbidden, "csrf validation failed")
				return
			}
			expected, ok := sm.CSRF(cookie.Value)
			provided := r.Header.Get(CSRFHeader)
			if !ok || len(provided) != len(expected) ||
				subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				writeError(w, http.StatusForbidden, "csrf validation failed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func OriginMiddleware(publicURL string) (func(http.Handler) http.Handler, error) {
	expected, err := normalizedOrigin(publicURL)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin, err := normalizedOrigin(r.Header.Get("Origin"))
			if err != nil || origin != expected {
				writeError(w, http.StatusForbidden, "origin validation failed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func normalizedOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid origin")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", errors.New("invalid origin scheme")
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}

func refreshSessionCookie(w http.ResponseWriter, token string, ttl time.Duration, secure bool) {
	maxAge := int((ttl + time.Second - 1) / time.Second)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode,
		MaxAge: maxAge, Expires: time.Now().Add(ttl),
	})
}

func setSessionCookie(w http.ResponseWriter, sm *SessionManager, secure bool) error {
	token, err := sm.Create()
	if err != nil {
		return err
	}
	refreshSessionCookie(w, token, sm.TTL(), secure)
	return nil
}

func LogoutHandler(sm *SessionManager, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(cookieName); err == nil {
			sm.Revoke(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Path: "/", HttpOnly: true,
			Secure: secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func CheckHandler(sm *SessionManager, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || !sm.Validate(cookie.Value) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"authenticated":false}`)
			return
		}
		refreshSessionCookie(w, cookie.Value, sm.TTL(), secureCookies)
		csrf, _ := sm.CSRF(cookie.Value)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"authenticated":true,"csrf_token":%q}`, csrf)
	}
}
