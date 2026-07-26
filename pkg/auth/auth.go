package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// SessionManager manages in-memory session tokens with expiry.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]time.Time
	ttl      time.Duration
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{sessions: make(map[string]time.Time), ttl: ttl}
}

func (sm *SessionManager) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	sm.mu.Lock()
	sm.sessions[token] = time.Now().Add(sm.ttl)
	sm.mu.Unlock()
	return token, nil
}

func (sm *SessionManager) Validate(token string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	expiry, ok := sm.sessions[token]
	if !ok || time.Now().After(expiry) {
		delete(sm.sessions, token)
		return false
	}
	sm.sessions[token] = time.Now().Add(sm.ttl)
	return true
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
	for token, expiry := range sm.sessions {
		if now.After(expiry) {
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
func Middleware(sm *SessionManager) func(http.Handler) http.Handler {
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
			next.ServeHTTP(w, r)
		})
	}
}

func setSessionCookie(w http.ResponseWriter, sm *SessionManager, secure bool) error {
	token, err := sm.Create()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 86400,
	})
	return nil
}

func LogoutHandler(sm *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(cookieName); err == nil {
			sm.Revoke(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Path: "/", HttpOnly: true,
			Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func CheckHandler(sm *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || !sm.Validate(cookie.Value) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"authenticated":false}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"authenticated":true}`)
	}
}
