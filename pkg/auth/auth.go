package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

// storedAuth is the on-disk format for auth credentials.
type storedAuth struct {
	PasswordHash string `json:"password_hash"`
}

// PasswordStore manages password hashing and verification with file persistence.
type PasswordStore struct {
	mu   sync.RWMutex
	path string
	hash []byte
}

func configDir() (string, error) {
	return paths.ConfigDir()
}

// NewPasswordStore creates a store backed by ~/.config/tmuxatlas/auth.json.
func NewPasswordStore() (*PasswordStore, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ps := &PasswordStore{
		path: filepath.Join(dir, "auth.json"),
	}
	// Try to load existing hash
	if data, err := os.ReadFile(ps.path); err == nil {
		var stored storedAuth
		if err := json.Unmarshal(data, &stored); err == nil && stored.PasswordHash != "" {
			ps.hash = []byte(stored.PasswordHash)
		}
	}
	return ps, nil
}

// HasPassword returns true if a password hash is stored.
func (ps *PasswordStore) HasPassword() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.hash) > 0
}

// SetPassword hashes and persists the given password.
func (ps *PasswordStore) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.hash = hash
	stored := storedAuth{PasswordHash: string(hash)}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ps.path, data, 0o600)
}

// Verify checks a password against the stored hash.
func (ps *PasswordStore) Verify(password string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if len(ps.hash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(ps.hash, []byte(password)) == nil
}

// SessionManager manages in-memory session tokens with expiry.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]time.Time
	ttl      time.Duration
}

// NewSessionManager creates a session manager with the given TTL.
func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]time.Time),
		ttl:      ttl,
	}
}

// Create generates a new session token.
func (sm *SessionManager) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[token] = time.Now().Add(sm.ttl)
	return token, nil
}

// Validate checks if a token is valid and refreshes its expiry (sliding window).
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

// Revoke removes a session token.
func (sm *SessionManager) Revoke(token string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, token)
}

// Cleanup removes expired sessions. Call periodically.
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

// isUnixSocket returns true if the request arrived over a unix socket.
func isUnixSocket(r *http.Request) bool {
	addr := r.Context().Value(http.LocalAddrContextKey)
	if addr == nil {
		return false
	}
	_, ok := addr.(*net.UnixAddr)
	return ok
}

// Middleware returns chi-compatible middleware that enforces session auth.
// Requests arriving over unix sockets bypass auth.
func Middleware(sm *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Unix socket connections are trusted (local CLI)
			if isUnixSocket(r) {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil || !sm.Validate(cookie.Value) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"unauthorized"}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetupHandler returns a handler for POST /api/auth/setup.
// Sets the initial password. Rejects if a password is already set.
func SetupHandler(ps *PasswordStore, sm *SessionManager, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ps.HasPassword() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":"password already set"}`)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
			http.Error(w, `{"error":"password is required"}`, http.StatusBadRequest)
			return
		}
		if len(req.Password) < 8 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"password must be at least 8 characters"}`)
			return
		}
		if err := ps.SetPassword(req.Password); err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		// Auto-login after setup
		token, err := sm.Create()
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookies,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400,
		})
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true}`)
	}
}

// LoginHandler returns a handler for POST /api/auth/login.
func LoginHandler(ps *PasswordStore, sm *SessionManager, secureCookies bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
			http.Error(w, `{"error":"password is required"}`, http.StatusBadRequest)
			return
		}
		if !ps.Verify(req.Password) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid password"}`)
			return
		}
		token, err := sm.Create()
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookies,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   86400, // 24h
		})
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true}`)
	}
}

// LogoutHandler returns a handler for POST /api/auth/logout.
func LogoutHandler(sm *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(cookieName); err == nil {
			sm.Revoke(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

// CheckHandler returns a handler for GET /api/auth/check.
func CheckHandler(sm *SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil || !sm.Validate(cookie.Value) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"authenticated":false}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"authenticated":true}`)
	}
}

// StatusHandler returns a handler for GET /api/auth/status.
// Always public — tells the frontend whether auth is enabled and if setup is needed.
func StatusHandler(authEnabled bool, ps *PasswordStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		needsSetup := false
		if authEnabled && ps != nil {
			needsSetup = !ps.HasPassword()
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"auth_required":%v,"needs_setup":%v}`, authEnabled, needsSetup)
	}
}
