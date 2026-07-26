package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/LosFurina/tmuxatlas/pkg/auth"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

const localToolEventBodyLimit = 4096

func newLocalRouter(tracker *toolevents.Tracker, peerMgr *peer.Manager, pairing *identity.PairingManager, passkeys *auth.PasskeyManager, health ...RuntimeHealth) http.Handler {
	mux := http.NewServeMux()
	if len(health) > 0 {
		mux.Handle("/health", healthHandler(health[0]))
	}
	mux.Handle("/api/tool-event", localToolEventHandler(tracker, peerMgr))
	if pairing != nil {
		mux.HandleFunc("/api/pair", func(w http.ResponseWriter, _ *http.Request) {
			code, err := pairing.Generate()
			if err != nil {
				http.Error(w, "pairing code unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code.Code, "expires_at": code.ExpiresAt})
		})
	}
	if passkeys != nil {
		mux.HandleFunc("/api/auth/bootstrap/rotate", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			token, err := passkeys.RotateBootstrapToken()
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"setup_token": token})
		})
	}
	if peerMgr != nil {
		mux.HandleFunc("/api/peers/revoke", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Name string `json:"name"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, localToolEventBodyLimit))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil || request.Name == "" {
				http.Error(w, "valid peer name is required", http.StatusBadRequest)
				return
			}
			revoked, err := peerMgr.RevokePeer(request.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": revoked.Name, "fingerprint": revoked.Fingerprint(), "revoked": true,
			})
		})
	}
	return mux
}

func localToolEventHandler(tracker *toolevents.Tracker, peerMgr *peer.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, localToolEventBodyLimit+1))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(body) > localToolEventBodyLimit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		var event toolevents.Event
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if event.Tool == "" || event.Status == "" || event.Session == "" {
			http.Error(w, "tool, status, and session are required", http.StatusBadRequest)
			return
		}
		if event.Host == "" && peerMgr != nil {
			event.Host = peerMgr.LocalID()
			event.HostName = peerMgr.LocalName()
		}
		tracker.Record(&event)
		w.WriteHeader(http.StatusNoContent)
	})
}
