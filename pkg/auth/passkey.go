package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

const ceremonyCookieName = "tmuxatlas_webauthn"

const maxPasskeyLabelRunes = 80
const maxCeremonies = 64
const bootstrapTTL = 10 * time.Minute

var (
	ErrPasskeyNotFound     = errors.New("passkey not found")
	ErrLastPasskey         = errors.New("the final passkey cannot be deleted")
	ErrInvalidPasskeyID    = errors.New("invalid passkey ID")
	ErrInvalidPasskeyLabel = errors.New("passkey label must be between 1 and 80 characters")
)

type StoredCredential struct {
	Label      string              `json:"label,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	LastUsedAt *time.Time          `json:"last_used_at,omitempty"`
	Credential webauthn.Credential `json:"credential"`
}

type PasskeyMetadata struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func encodeCredentialID(id []byte) string {
	return base64.RawURLEncoding.EncodeToString(id)
}

func decodeCredentialID(value string) ([]byte, error) {
	id, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(id) == 0 {
		return nil, ErrInvalidPasskeyID
	}
	return id, nil
}

func validatePasskeyLabel(value string, allowEmpty bool) (string, error) {
	label := strings.TrimSpace(value)
	if label == "" && allowEmpty {
		return "", nil
	}
	if label == "" || utf8.RuneCountInString(label) > maxPasskeyLabelRunes {
		return "", ErrInvalidPasskeyLabel
	}
	return label, nil
}

type passkeyData struct {
	Version     int                `json:"version"`
	RPID        string             `json:"rp_id"`
	Origin      string             `json:"origin"`
	UserID      []byte             `json:"user_id"`
	Credentials []StoredCredential `json:"credentials"`
}

// PasskeyStore persists public WebAuthn credential records. The authenticator's
// private key never leaves the device or password manager.
type PasskeyStore struct {
	mu   sync.RWMutex
	path string
	data passkeyData
}

func NewPasskeyStore(rpID, origin string) (*PasskeyStore, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &PasskeyStore{path: filepath.Join(dir, "passkeys.json")}
	if raw, err := os.ReadFile(s.path); err == nil {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, fmt.Errorf("decode passkey store: %w", err)
		}
		if len(s.data.Credentials) > 0 && (s.data.RPID != rpID || s.data.Origin != origin) {
			return nil, fmt.Errorf("passkeys are bound to %s (%s), but --public-url resolves to %s (%s)", s.data.Origin, s.data.RPID, origin, rpID)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(s.data.UserID) == 0 {
		s.data = passkeyData{Version: 1, RPID: rpID, Origin: origin, UserID: randomBytes(32)}
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func randomBytes(size int) []byte {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic("system random source unavailable: " + err.Error())
	}
	return b
}

func (s *PasskeyStore) saveLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".passkeys-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func (s *PasskeyStore) HasCredentials() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Credentials) > 0
}

func (s *PasskeyStore) List() []PasskeyMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PasskeyMetadata, len(s.data.Credentials))
	for i, stored := range s.data.Credentials {
		result[i] = PasskeyMetadata{
			ID: encodeCredentialID(stored.Credential.ID), Label: stored.Label,
			CreatedAt: stored.CreatedAt, LastUsedAt: stored.LastUsedAt,
		}
	}
	return result
}

func (s *PasskeyStore) Rename(encodedID, value string) error {
	id, err := decodeCredentialID(encodedID)
	if err != nil {
		return err
	}
	label, err := validatePasskeyLabel(value, false)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Credentials {
		if bytes.Equal(s.data.Credentials[i].Credential.ID, id) {
			previous := s.data.Credentials[i].Label
			s.data.Credentials[i].Label = label
			if err := s.saveLocked(); err != nil {
				s.data.Credentials[i].Label = previous
				return err
			}
			return nil
		}
	}
	return ErrPasskeyNotFound
}

func (s *PasskeyStore) Delete(encodedID string) error {
	id, err := decodeCredentialID(encodedID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i := range s.data.Credentials {
		if bytes.Equal(s.data.Credentials[i].Credential.ID, id) {
			index = i
			break
		}
	}
	if index < 0 {
		return ErrPasskeyNotFound
	}
	if len(s.data.Credentials) <= 1 {
		return ErrLastPasskey
	}
	previous := s.data.Credentials
	next := make([]StoredCredential, 0, len(previous)-1)
	next = append(next, previous[:index]...)
	next = append(next, previous[index+1:]...)
	s.data.Credentials = next
	if err := s.saveLocked(); err != nil {
		s.data.Credentials = previous
		return err
	}
	return nil
}

func (s *PasskeyStore) snapshot() passkeyUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	credentials := make([]webauthn.Credential, len(s.data.Credentials))
	for i := range s.data.Credentials {
		credentials[i] = s.data.Credentials[i].Credential
	}
	return passkeyUser{id: append([]byte(nil), s.data.UserID...), credentials: credentials}
}

func (s *PasskeyStore) add(label string, credential *webauthn.Credential) error {
	label, err := validatePasskeyLabel(label, true)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.Credentials {
		if bytes.Equal(existing.Credential.ID, credential.ID) {
			return errors.New("passkey already registered")
		}
	}
	s.data.Credentials = append(s.data.Credentials, StoredCredential{
		Label: label, CreatedAt: time.Now().UTC(), Credential: *credential,
	})
	return s.saveLocked()
}

func (s *PasskeyStore) update(credential *webauthn.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Credentials {
		if bytes.Equal(s.data.Credentials[i].Credential.ID, credential.ID) {
			now := time.Now().UTC()
			s.data.Credentials[i].Credential = *credential
			s.data.Credentials[i].LastUsedAt = &now
			return s.saveLocked()
		}
	}
	return ErrPasskeyNotFound
}

type passkeyUser struct {
	id          []byte
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return u.id }
func (u passkeyUser) WebAuthnName() string                       { return "tmuxatlas-admin" }
func (u passkeyUser) WebAuthnDisplayName() string                { return "TmuxAtlas Admin" }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

type ceremony struct {
	kind      string
	label     string
	session   *webauthn.SessionData
	expires   time.Time
	bootstrap bool
}

type PasskeyManager struct {
	mu               sync.Mutex
	webAuthn         *webauthn.WebAuthn
	store            *PasskeyStore
	sessions         *SessionManager
	ceremonies       map[string]ceremony
	bootstrapHash    []byte
	bootstrap        string
	bootstrapExpires time.Time
	secureCookies    bool
	rpID             string
	origin           string
}

func NewPasskeyManager(publicURL string, sessions *SessionManager) (*PasskeyManager, error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("invalid public URL for passkeys")
	}
	origin := u.Scheme + "://" + u.Host
	store, err := NewPasskeyStore(u.Hostname(), origin)
	if err != nil {
		return nil, err
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "TmuxAtlas",
		RPID:          u.Hostname(),
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, err
	}
	m := &PasskeyManager{
		webAuthn: wa, store: store, sessions: sessions,
		ceremonies: make(map[string]ceremony), secureCookies: u.Scheme == "https",
		rpID: u.Hostname(), origin: origin,
	}
	if !store.HasCredentials() {
		m.bootstrap = base64.RawURLEncoding.EncodeToString(randomBytes(32))
		m.bootstrapHash = bootstrapDigest(m.bootstrap)
		m.bootstrapExpires = time.Now().Add(bootstrapTTL)
	}
	return m, nil
}

func bootstrapDigest(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func (m *PasskeyManager) BootstrapToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	token := m.bootstrap
	m.bootstrap = ""
	return token
}

func (m *PasskeyManager) RotateBootstrapToken() (string, error) {
	if m.store.HasCredentials() {
		return "", errors.New("bootstrap rotation is unavailable after a passkey is registered")
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	m.mu.Lock()
	m.bootstrap = ""
	m.bootstrapHash = bootstrapDigest(token)
	m.bootstrapExpires = time.Now().Add(bootstrapTTL)
	m.mu.Unlock()
	return token, nil
}
func (m *PasskeyManager) HasCredentials() bool { return m.store.HasCredentials() }
func (m *PasskeyManager) RPID() string         { return m.rpID }
func (m *PasskeyManager) Origin() string       { return m.origin }

func (m *PasskeyManager) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	return err == nil && m.sessions.Validate(cookie.Value)
}

func (m *PasskeyManager) authorizeRegistration(r *http.Request, supplied string) bool {
	if m.store.HasCredentials() {
		return m.authenticated(r)
	}
	got := bootstrapDigest(supplied)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.bootstrapHash) == 0 || time.Now().After(m.bootstrapExpires) ||
		subtle.ConstantTimeCompare(got, m.bootstrapHash) != 1 {
		return false
	}
	// Bind the token to the one registration ceremony. It cannot be replayed
	// after success, failure, cancellation, or timeout.
	m.bootstrapHash = nil
	return true
}

func (m *PasskeyManager) putCeremony(w http.ResponseWriter, value ceremony) error {
	token := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	value.expires = time.Now().Add(5 * time.Minute)
	m.mu.Lock()
	for key, existing := range m.ceremonies {
		if time.Now().After(existing.expires) {
			delete(m.ceremonies, key)
		}
	}
	if len(m.ceremonies) >= maxCeremonies {
		m.mu.Unlock()
		return errors.New("too many passkey ceremonies")
	}
	m.ceremonies[token] = value
	m.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: ceremonyCookieName, Value: token, Path: "/api/auth/passkey",
		HttpOnly: true, Secure: m.secureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 300,
	})
	return nil
}

func (m *PasskeyManager) takeCeremony(r *http.Request, kind string) (ceremony, error) {
	cookie, err := r.Cookie(ceremonyCookieName)
	if err != nil {
		return ceremony{}, errors.New("passkey ceremony expired")
	}
	m.mu.Lock()
	value, ok := m.ceremonies[cookie.Value]
	delete(m.ceremonies, cookie.Value)
	m.mu.Unlock()
	if !ok || value.kind != kind || time.Now().After(value.expires) {
		return ceremony{}, errors.New("passkey ceremony expired")
	}
	return value, nil
}

func (m *PasskeyManager) BeginRegistrationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SetupToken string `json:"setup_token"`
			Label      string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if !m.authorizeRegistration(r, req.SetupToken) {
			writeError(w, http.StatusUnauthorized, "invalid setup token or session")
			return
		}
		user := m.store.snapshot()
		options, session, err := m.webAuthn.BeginMediatedRegistration(
			user,
			protocol.MediationDefault,
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				UserVerification: protocol.VerificationRequired,
			}),
			webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
			webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not begin passkey registration")
			return
		}
		if err := m.putCeremony(w, ceremony{kind: "register", label: req.Label, session: session}); err != nil {
			writeError(w, http.StatusServiceUnavailable, "passkey ceremony capacity reached")
			return
		}
		writeJSON(w, http.StatusOK, options)
	}
}

func (m *PasskeyManager) ListHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"passkeys": m.store.List()})
	}
}

func (m *PasskeyManager) RenameHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Label string `json:"label"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		err := m.store.Rename(chi.URLParam(r, "credentialID"), req.Label)
		switch {
		case errors.Is(err, ErrInvalidPasskeyID), errors.Is(err, ErrInvalidPasskeyLabel):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrPasskeyNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, "could not rename passkey")
		default:
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		}
	}
}

func (m *PasskeyManager) DeleteHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := m.store.Delete(chi.URLParam(r, "credentialID"))
		switch {
		case errors.Is(err, ErrInvalidPasskeyID):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrPasskeyNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrLastPasskey):
			writeError(w, http.StatusConflict, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, "could not delete passkey")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (m *PasskeyManager) FinishRegistrationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := m.takeCeremony(r, "register")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hadCredentials := m.store.HasCredentials()
		credential, err := m.webAuthn.FinishRegistration(m.store.snapshot(), *value.session, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "passkey verification failed")
			return
		}
		if err := m.store.add(value.label, credential); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		m.mu.Lock()
		m.bootstrap = ""
		m.bootstrapHash = nil
		m.bootstrapExpires = time.Time{}
		m.mu.Unlock()
		if !hadCredentials {
			if err := setSessionCookie(w, m.sessions, m.secureCookies); err != nil {
				writeError(w, http.StatusInternalServerError, "could not create session")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (m *PasskeyManager) BeginLoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.store.HasCredentials() {
			writeError(w, http.StatusPreconditionFailed, "passkey setup required")
			return
		}
		options, session, err := m.webAuthn.BeginDiscoverableMediatedLogin(
			protocol.MediationDefault,
			webauthn.WithUserVerification(protocol.VerificationRequired),
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not begin passkey login")
			return
		}
		if err := m.putCeremony(w, ceremony{kind: "login", session: session}); err != nil {
			writeError(w, http.StatusServiceUnavailable, "passkey ceremony capacity reached")
			return
		}
		writeJSON(w, http.StatusOK, options)
	}
}

func (m *PasskeyManager) FinishLoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := m.takeCeremony(r, "login")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		_, credential, err := m.webAuthn.FinishPasskeyLogin(
			func(rawID, userHandle []byte) (webauthn.User, error) {
				user := m.store.snapshot()
				if !bytes.Equal(user.id, userHandle) {
					return nil, errors.New("unknown passkey user")
				}
				for _, candidate := range user.credentials {
					if bytes.Equal(candidate.ID, rawID) {
						return user, nil
					}
				}
				return nil, errors.New("unknown passkey")
			},
			*value.session,
			r,
		)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "passkey verification failed")
			return
		}
		if err := m.store.update(credential); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update passkey")
			return
		}
		if err := setSessionCookie(w, m.sessions, m.secureCookies); err != nil {
			writeError(w, http.StatusInternalServerError, "could not create session")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func StatusHandler(authEnabled bool, manager *PasskeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		needsSetup := authEnabled && manager != nil && !manager.HasCredentials()
		status := map[string]any{
			"auth_required": authEnabled,
			"needs_setup":   needsSetup,
			"passkey":       authEnabled,
		}
		if manager != nil {
			status["rp_id"] = manager.RPID()
			status["origin"] = manager.Origin()
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
