package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/agentcheck"
	"github.com/LosFurina/tmuxatlas/pkg/auth"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/httpguard"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/ingress"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/preferences"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/stats"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
	"github.com/LosFurina/tmuxatlas/pkg/webpush"
	"github.com/LosFurina/tmuxatlas/pkg/ws"
)

type Options struct {
	Role             string
	Deployment       string
	ListenAddress    string
	PublicURL        string
	SecureCookies    bool
	SocketPath       string
	Client           *tmux.Client
	StateMgr         *state.Manager
	StateCoordinator *state.Coordinator
	Tracker          *toolevents.Tracker
	ActivityTracker  *activity.Tracker
	PushKeys         *webpush.VAPIDKeys
	PushStore        *webpush.Store
	PrefStore        *preferences.Store
	AuthEnabled      bool
	PasskeyManager   *auth.PasskeyManager
	SessionMgr       *auth.SessionManager
	PeerMgr          *peer.Manager
	PeerHandler      *peer.Handler
	PairingMgr       *identity.PairingManager
	PTYRelay         *peer.PTYRelay
	Detector         *toolevents.Detector
	LocalOnly        bool
}

func writeRuntimeError(w http.ResponseWriter, err error) {
	var runtimeError peer.RuntimeError
	if !errors.As(err, &runtimeError) {
		runtimeError = peer.RuntimeError{Code: peer.ErrorExecutionFailed}
	}
	status := http.StatusInternalServerError
	switch runtimeError.Code {
	case peer.ErrorInvalidTarget:
		status = http.StatusBadRequest
	case peer.ErrorNotFound:
		status = http.StatusNotFound
	case peer.ErrorPeerOffline, peer.ErrorPeerRevoked:
		status = http.StatusServiceUnavailable
	case peer.ErrorProtocolIncompatible, peer.ErrorCapabilityUnsupported,
		peer.ErrorRequestConflict, peer.ErrorExecutionUnknown:
		status = http.StatusConflict
	case peer.ErrorQueueFull, peer.ErrorResourceExhausted:
		status = http.StatusTooManyRequests
	case peer.ErrorTimeout:
		status = http.StatusGatewayTimeout
	case peer.ErrorStaleGeneration:
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(runtimeError)
}

// handleRemoteSession handles a terminal session request for a remote peer.
// It tells the peer to spawn a PTY, then bridges the browser WS to the peer's PTY WS.
func handleRemoteSession(w http.ResponseWriter, r *http.Request, opts *Options, hostID string) {
	sessionName := r.URL.Query().Get("name")
	if sessionName == "" {
		http.Error(w, "missing session name", http.StatusBadRequest)
		return
	}

	cols := uint16(80)
	rows := uint16(24)
	if c := r.URL.Query().Get("cols"); c != "" {
		if v, err := strconv.ParseUint(c, 10, 16); err == nil && v > 0 {
			cols = uint16(v)
		}
	}
	if rv := r.URL.Query().Get("rows"); rv != "" {
		if v, err := strconv.ParseUint(rv, 10, 16); err == nil && v > 0 {
			rows = uint16(v)
		}
	}

	// Get the peer connection
	peerConn := opts.PeerMgr.GetPeerConnection(hostID)
	if peerConn == nil {
		http.Error(w, "peer not connected", http.StatusBadGateway)
		return
	}
	target := peer.SessionTarget{HostID: hostID, Session: sessionName}
	if err := target.Validate(); err != nil || !opts.PeerMgr.HasSession(hostID, sessionName) {
		http.Error(w, "invalid or unknown session target", http.StatusNotFound)
		return
	}
	if !peerConn.Supports(peer.CapabilityPTYControl) {
		http.Error(w, "peer does not support PTY control", http.StatusConflict)
		return
	}

	// Upgrade browser to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin:    ws.CheckSameOrigin,
		ReadBufferSize: 1024, WriteBufferSize: 1024 * 16,
	}
	browserWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer browserWS.Close()

	// Generate stream ID and register pending relay
	owner, err := opts.PTYRelay.RegisterPending(peerConn, target, browserWS)
	if err != nil {
		return
	}
	defer owner.Teardown("browser-handler-exit")

	// Tell the peer to open a PTY
	msg, _ := peer.NewMessage(peer.MsgPTYOpen, peer.PTYOpenPayload{
		StreamID: owner.StreamID, AttachToken: owner.AttachToken,
		Generation: peerConn.Generation, Target: target, Cols: cols, Rows: rows,
	})
	if err := peerConn.Send(r.Context(), msg); err != nil {
		return
	}

	// Wait for the peer to connect its PTY WebSocket
	select {
	case <-owner.Ready():
		owner.Bridge()
	case <-time.After(15 * time.Second):
		return
	case <-peerConn.Done():
		return
	}
}

func Run(ctx context.Context, opts *Options) error {
	logger := logrus.WithField("component", "server")

	coordinator := opts.StateCoordinator
	if coordinator == nil {
		var err error
		coordinator, err = state.NewCoordinator()
		if err != nil {
			return fmt.Errorf("initialize canonical state: %w", err)
		}
		defer coordinator.Close()
		opts.StateCoordinator = coordinator
	}
	if opts.PeerMgr != nil {
		if _, err := coordinator.ReplaceHosts(ctx, opts.PeerMgr.StateHostSnapshots()); err != nil {
			return fmt.Errorf("initialize canonical host projection: %w", err)
		}
		if err := syncFleetHealth(ctx, coordinator, opts.PeerMgr.GetHosts(), time.Now()); err != nil {
			return fmt.Errorf("initialize fleet health: %w", err)
		}
		stateEvents := opts.PeerMgr.Subscribe()
		defer opts.PeerMgr.Unsubscribe(stateEvents)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-stateEvents:
					if !ok {
						return
					}
					if _, err := coordinator.ReplaceHosts(ctx, opts.PeerMgr.StateHostSnapshots()); err != nil &&
						!errors.Is(err, context.Canceled) {
						logger.WithError(err).Warn("failed to refresh canonical host projection")
					}
					if err := syncFleetHealth(ctx, coordinator, opts.PeerMgr.GetHosts(), time.Now()); err != nil &&
						!errors.Is(err, context.Canceled) {
						logger.WithError(err).Warn("failed to refresh fleet health")
					}
				}
			}
		}()
	}
	if opts.PrefStore != nil {
		if _, err := coordinator.SetMetadata(ctx, "preferences", opts.PrefStore.Get()); err != nil {
			return fmt.Errorf("initialize canonical preferences: %w", err)
		}
	}
	if _, err := coordinator.SetMetadata(ctx, "server", map[string]string{
		"version": common.VERSION,
		"commit":  common.COMMIT,
	}); err != nil {
		return fmt.Errorf("initialize canonical server metadata: %w", err)
	}
	localHostID := ""
	if opts.PeerMgr != nil {
		localHostID = opts.PeerMgr.LocalID()
	}
	if opts.Tracker != nil {
		toolEvents := opts.Tracker.Subscribe()
		defer opts.Tracker.Unsubscribe(toolEvents)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-toolEvents:
					if !ok {
						return
					}
					if _, err := coordinator.ApplyToolEvent(ctx, event, localHostID); err != nil &&
						!errors.Is(err, context.Canceled) {
						logger.WithError(err).Warn("failed to commit canonical tool event")
					}
				}
			}
		}()
	}
	if opts.ActivityTracker != nil {
		collectActivity := func() []*activity.Snapshot {
			snapshots := opts.ActivityTracker.GetAll()
			if opts.PeerMgr != nil && !opts.LocalOnly {
				snapshots = append(snapshots, opts.PeerMgr.GetAllActivity()...)
			}
			return snapshots
		}
		if _, err := coordinator.ReplaceActivity(ctx, collectActivity(), localHostID); err != nil {
			return fmt.Errorf("initialize canonical activity: %w", err)
		}
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if _, err := coordinator.ReplaceActivity(ctx, collectActivity(), localHostID); err != nil &&
						!errors.Is(err, context.Canceled) {
						logger.WithError(err).Warn("failed to refresh canonical activity")
					}
				}
			}
		}()
	}

	publicURL := opts.PublicURL
	if publicURL == "" {
		publicURL = "http://localhost:7654"
	}
	originMiddleware, err := auth.OriginMiddleware(publicURL)
	if err != nil {
		return fmt.Errorf("public URL origin: %w", err)
	}
	hostValidation, err := hostMiddleware(publicURL)
	if err != nil {
		return fmt.Errorf("public URL host: %w", err)
	}
	ingressPolicy, err := ingress.NewPolicy(ingress.DefaultConfig(), nil)
	if err != nil {
		return fmt.Errorf("ingress policy: %w", err)
	}
	var actionRouter *peer.ActionRouter
	if opts.PeerMgr != nil {
		var localExecutor peer.RuntimeExecutor
		if opts.Client != nil {
			localExecutor = peer.NewTmuxRuntimeExecutor(opts.Client)
		}
		actionRouter = peer.NewActionRouter(opts.PeerMgr, localExecutor, 10*time.Second)
	}

	publicRouter := chi.NewRouter()
	publicRouter.Use(chimiddleware.Recoverer)
	publicRouter.Use(chimiddleware.StripSlashes)
	publicRouter.Use(chimiddleware.RequestID)
	publicRouter.Use(hostValidation)
	publicRouter.Use(httpguard.GlobalBodyLimitMiddleware(httpguard.GlobalBodyLimit))

	// API routes
	publicRouter.Route("/api", func(r chi.Router) {
		// Public auth endpoints (no middleware)
		r.Get("/auth/status", auth.StatusHandler(opts.AuthEnabled, opts.PasskeyManager))
		if opts.AuthEnabled {
			preSessionJSON := []func(http.Handler) http.Handler{
				admissionMiddleware(ingressPolicy, ingress.CategoryWebAuthn),
				originMiddleware,
				httpguard.BodyReadDeadline(15 * time.Second),
				httpguard.JSONBody(httpguard.WebAuthnLimit),
			}
			r.With(preSessionJSON...).Post("/auth/passkey/register/begin", opts.PasskeyManager.BeginRegistrationHandler())
			r.With(preSessionJSON...).Post("/auth/passkey/register/finish", opts.PasskeyManager.FinishRegistrationHandler())
			r.With(preSessionJSON...).Post("/auth/passkey/login/begin", opts.PasskeyManager.BeginLoginHandler())
			r.With(preSessionJSON...).Post("/auth/passkey/login/finish", opts.PasskeyManager.FinishLoginHandler())
			r.With(
				auth.Middleware(opts.SessionMgr, opts.SecureCookies),
				originMiddleware,
				auth.CSRFMiddleware(opts.SessionMgr),
			).Post("/auth/logout", auth.LogoutHandler(opts.SessionMgr, opts.SecureCookies))
			r.Get("/auth/check", auth.CheckHandler(opts.SessionMgr, opts.SecureCookies))
		}

		// Version endpoint — public, no auth required
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"version": common.VERSION,
				"commit":  common.COMMIT,
			})
		})

		// Protected API routes
		r.Group(func(r chi.Router) {
			mutations := r
			if opts.AuthEnabled {
				r.Use(auth.Middleware(opts.SessionMgr, opts.SecureCookies))
				r.Get("/auth/passkeys", opts.PasskeyManager.ListHandler())
				mutations = r.With(originMiddleware, auth.CSRFMiddleware(opts.SessionMgr))
				mutations.With(
					httpguard.BodyReadDeadline(10*time.Second),
					httpguard.JSONBody(httpguard.SmallJSONLimit),
				).Patch("/auth/passkeys/{credentialID}", opts.PasskeyManager.RenameHandler())
				mutations.Delete("/auth/passkeys/{credentialID}", opts.PasskeyManager.DeleteHandler())
			}

			// Agent status — check which agents are installed/configured
			r.Get("/agent-status", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if opts.Role == "hub" {
					json.NewEncoder(w).Encode(agentcheck.StatusResult{
						Agents:       []agentcheck.AgentStatus{},
						SetupCommand: "",
					})
					return
				}
				json.NewEncoder(w).Encode(agentcheck.CheckAgents())
			})

			r.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
				snapshot, err := coordinator.Snapshot(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(snapshot.State.SessionViews())
			})

			r.Get("/hosts", func(w http.ResponseWriter, r *http.Request) {
				snapshot, err := coordinator.Snapshot(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(snapshot.State.HostViews())
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/new", func(w http.ResponseWriter, r *http.Request) {
				handleSessionNew(w, r, actionRouter, opts)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/rename", func(w http.ResponseWriter, r *http.Request) {
				handleSessionRename(w, r, actionRouter, opts)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/select-window", func(w http.ResponseWriter, r *http.Request) {
				handleSessionSelectWindow(w, r, actionRouter)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/session/select-pane", func(w http.ResponseWriter, r *http.Request) {
				handleSessionSelectPane(w, r, actionRouter)
			})

			// Tool event query/management (auth-protected)
			r.Get("/tool-events", func(w http.ResponseWriter, r *http.Request) {
				session := r.URL.Query().Get("session")
				snapshot, err := coordinator.Snapshot(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				events := snapshot.State.ToolEventViews()
				if session != "" {
					filtered := events[:0]
					for _, event := range events {
						if event.Session == session {
							filtered = append(filtered, event)
						}
					}
					events = filtered
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(events)
			})

			mutations.Delete("/tool-events", func(w http.ResponseWriter, r *http.Request) {
				if opts.Tracker != nil {
					opts.Tracker.ClearAll()
				}
				if _, err := coordinator.ClearToolEvents(r.Context(), "", "", 0, ""); err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Delete("/tool-event", func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Host    string `json:"host"`
					Session string `json:"session"`
					Window  int    `json:"window"`
					Pane    string `json:"pane"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Session == "" {
					http.Error(w, "session is required", http.StatusBadRequest)
					return
				}
				if opts.Tracker != nil {
					opts.Tracker.Clear(req.Host, req.Session, req.Window, req.Pane)
				}
				if _, err := coordinator.ClearToolEvents(
					r.Context(), req.Host, req.Session, req.Window, req.Pane,
				); err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			// Stats endpoint — aggregate overview data
			r.Get("/stats", func(w http.ResponseWriter, r *http.Request) {
				snapshot, err := coordinator.Snapshot(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				sessions := snapshot.State.SessionViews()
				var allPanes []*tmux.Pane
				for _, session := range sessions {
					for _, window := range session.Windows {
						allPanes = append(allPanes, window.Panes...)
					}
				}

				agentCommands := map[string]bool{
					"claude": true, "codex": true, "copilot": true, "opencode": true, "agy": true,
				}
				totalWindows := 0
				attachedSessions := 0
				agentPanes := 0
				for _, s := range sessions {
					if s.Attached {
						attachedSessions++
					}
					totalWindows += len(s.Windows)
				}

				// Build a set of panes with known agent tool events (from hooks
				// or process-tree detection). This catches agents like codex and
				// copilot that show up as "node" in pane_current_command.
				toolEvents := snapshot.State.ToolEventViews()
				agentEventPanes := make(map[string]bool)
				for _, evt := range toolEvents {
					if evt.Pane != "" {
						agentEventPanes[evt.Pane] = true
					}
				}
				// Also include panes detected via process tree inspection
				if opts.Detector != nil {
					for paneID := range opts.Detector.DetectedPanes() {
						agentEventPanes[paneID] = true
					}
				}

				for _, p := range allPanes {
					if agentCommands[p.CurrentCommand] || agentEventPanes[p.ID] {
						agentPanes++
					}
				}
				waitingAgents := 0
				errorAgents := 0
				for _, evt := range toolEvents {
					switch evt.Status {
					case "waiting":
						waitingAgents++
					case "error":
						errorAgents++
					}
				}

				systemStats := map[string]interface{}{}
				if opts.Role != "hub" {
					systemStats = stats.SystemStats()
				}
				result := map[string]interface{}{
					"sessions": map[string]int{
						"total":    len(sessions),
						"attached": attachedSessions,
						"detached": len(sessions) - attachedSessions,
					},
					"windows":     totalWindows,
					"panes":       len(allPanes),
					"agent_panes": agentPanes,
					"agents": map[string]int{
						"active":  agentPanes,
						"waiting": waitingAgents,
						"error":   errorAgents,
					},
					"processes": stats.ProcessCountsFromSessions(sessions),
					"system":    systemStats,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
			})

			// Activity endpoints
			r.Get("/activity", func(w http.ResponseWriter, r *http.Request) {
				session := r.URL.Query().Get("session")
				snapshot, err := coordinator.Snapshot(r.Context())
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				snapshots := snapshot.State.ActivityViews()
				w.Header().Set("Content-Type", "application/json")
				if session != "" {
					for _, activitySnapshot := range snapshots {
						if activitySnapshot.SessionName == session {
							json.NewEncoder(w).Encode(activitySnapshot)
							return
						}
					}
					json.NewEncoder(w).Encode(nil)
				} else {
					json.NewEncoder(w).Encode(snapshots)
				}
			})

			// Push notification endpoints
			r.Get("/push/vapid-key", func(w http.ResponseWriter, r *http.Request) {
				if opts.PushKeys == nil {
					http.Error(w, "push notifications not configured", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"public_key": opts.PushKeys.PublicKey,
				})
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.WebAuthnLimit)).Post("/push/subscribe", func(w http.ResponseWriter, r *http.Request) {
				if opts.PushStore == nil {
					http.Error(w, "push notifications not configured", http.StatusServiceUnavailable)
					return
				}
				var sub wp.Subscription
				if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || sub.Endpoint == "" {
					http.Error(w, "invalid subscription", http.StatusBadRequest)
					return
				}
				if err := opts.PushStore.Add(&sub); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Post("/push/unsubscribe", func(w http.ResponseWriter, r *http.Request) {
				if opts.PushStore == nil {
					http.Error(w, "push notifications not configured", http.StatusServiceUnavailable)
					return
				}
				var req struct {
					Endpoint string `json:"endpoint"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Endpoint == "" {
					http.Error(w, "endpoint is required", http.StatusBadRequest)
					return
				}
				if err := opts.PushStore.Remove(req.Endpoint); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			// Preferences endpoints
			r.Get("/preferences", func(w http.ResponseWriter, r *http.Request) {
				var prefs *preferences.Preferences
				if opts.PrefStore != nil {
					prefs = opts.PrefStore.Get()
				} else {
					prefs = preferences.Default()
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(prefs)
			})

			mutations.With(httpguard.BodyReadDeadline(10*time.Second), httpguard.JSONBody(httpguard.SmallJSONLimit)).Put("/preferences", func(w http.ResponseWriter, r *http.Request) {
				if opts.PrefStore == nil {
					http.Error(w, "preferences not available", http.StatusServiceUnavailable)
					return
				}
				var prefs preferences.Preferences
				if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
					http.Error(w, "invalid JSON", http.StatusBadRequest)
					return
				}
				if err := opts.PrefStore.Update(&prefs); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if _, err := coordinator.SetMetadata(r.Context(), "preferences", &prefs); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(&prefs)
			})

			// Pairing code generation (for the hub to generate codes)
			mutations.With(admissionMiddleware(ingressPolicy, ingress.CategoryPairing)).Post("/pair", func(w http.ResponseWriter, r *http.Request) {
				if opts.PairingMgr == nil {
					http.Error(w, "pairing not available", http.StatusServiceUnavailable)
					return
				}
				code, err := opts.PairingMgr.Generate()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":       code.Code,
					"expires_at": code.ExpiresAt,
				})
			})
		})
	})

	// WebSocket routes (protected by auth if enabled)
	hub := ws.NewHub(opts.StateMgr, opts.Tracker)
	hub.SetCoordinator(coordinator)
	if opts.ActivityTracker != nil {
		var peerActivity ws.ActivitySource
		localHostID := ""
		localOnly := opts.LocalOnly
		if opts.PeerMgr != nil {
			peerActivity = opts.PeerMgr
			localHostID = opts.PeerMgr.LocalID()
		}
		hub.SetActivityTracker(opts.ActivityTracker, peerActivity, localHostID, localOnly)
	}
	go hub.Run()

	var ptyHandler *ws.PTYTerminalHandler
	if opts.Client != nil {
		ptyHandler = ws.NewPTYTerminalHandler(opts.Client.TmuxPath(), opts.ActivityTracker)
	}

	if opts.AuthEnabled {
		authMw := auth.Middleware(opts.SessionMgr, opts.SecureCookies)
		publicRouter.With(authMw, originMiddleware, admissionMiddleware(ingressPolicy, ingress.CategoryWSTerminal)).Get("/ws/events", hub.HandleEvents)
		publicRouter.With(authMw, originMiddleware, admissionMiddleware(ingressPolicy, ingress.CategoryWSTerminal)).Get("/ws/session", func(w http.ResponseWriter, req *http.Request) {
			hostID := req.URL.Query().Get("host")
			if opts.PeerMgr == nil || hostID == "" || !opts.PeerMgr.HasHost(hostID) {
				http.Error(w, "explicit known host is required", http.StatusBadRequest)
				return
			}
			if !opts.PeerMgr.IsLocal(hostID) {
				handleRemoteSession(w, req, opts, hostID)
				return
			}
			if ptyHandler == nil {
				writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorCapabilityUnsupported})
				return
			}
			ptyHandler.HandleSession(w, req)
		})
	} else {
		publicRouter.With(originMiddleware, admissionMiddleware(ingressPolicy, ingress.CategoryWSTerminal)).Get("/ws/events", hub.HandleEvents)
		publicRouter.With(originMiddleware, admissionMiddleware(ingressPolicy, ingress.CategoryWSTerminal)).Get("/ws/session", func(w http.ResponseWriter, req *http.Request) {
			hostID := req.URL.Query().Get("host")
			if opts.PeerMgr == nil || hostID == "" || !opts.PeerMgr.HasHost(hostID) {
				http.Error(w, "explicit known host is required", http.StatusBadRequest)
				return
			}
			if !opts.PeerMgr.IsLocal(hostID) {
				handleRemoteSession(w, req, opts, hostID)
				return
			}
			if ptyHandler == nil {
				writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorCapabilityUnsupported})
				return
			}
			ptyHandler.HandleSession(w, req)
		})
	}

	// Peer WebSocket routes (no browser auth — peers use their own challenge-response)
	if opts.PeerHandler != nil {
		publicRouter.With(admissionMiddleware(ingressPolicy, ingress.CategoryWSPeer)).Get("/ws/peer", opts.PeerHandler.HandlePeer)
		publicRouter.With(
			admissionMiddleware(ingressPolicy, ingress.CategoryPairing),
			httpguard.BodyReadDeadline(10*time.Second),
			httpguard.JSONBody(httpguard.PairingLimit),
		).Post("/api/pair/complete", opts.PeerHandler.HandlePairing)
	}
	if opts.PTYRelay != nil {
		publicRouter.With(admissionMiddleware(ingressPolicy, ingress.CategoryWSPeerPTY)).Get("/ws/peer-pty", opts.PTYRelay.HandlePeerPTY)
	}

	// Serve embedded frontend
	sub, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return fmt.Errorf("frontend fs: %w", err)
	}
	publicRouter.Get("/*", frontendHandler(sub).ServeHTTP)

	publicServer := &http.Server{
		Handler:           publicRouter,
		ErrorLog:          log.New(logger.WriterLevel(logrus.WarnLevel), "", 0),
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    32 << 10,
		// Note: ReadTimeout and WriteTimeout are intentionally omitted.
		// They apply to the underlying net.Conn and would kill long-lived
		// WebSocket connections after the timeout period.
	}

	serverErr := make(chan error, 2)

	listenAddress := opts.ListenAddress
	if listenAddress == "" {
		listenAddress = "127.0.0.1:7654"
	}
	tcpListener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("tcp listen: %w", err)
	}

	go func() {
		serveErr := publicServer.Serve(tcpListener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.WithError(serveErr).Error("tcp listen error")
			serverErr <- serveErr
		}
	}()

	logger.WithField("listen", listenAddress).Info("starting TmuxAtlas HTTP origin")
	logger.Infof("open %s in your browser", publicURL)
	if opts.AuthEnabled {
		logger.Info("authentication is enabled")
	}

	// Start Unix socket listener for local notify CLI
	var unixListener net.Listener
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = socket.DefaultPath()
	}
	unixListener, err = socket.Listen(socketPath)
	if err != nil {
		logger.WithError(err).Warn("failed to listen on unix socket, notify via socket will be unavailable")
	} else {
		role := opts.Role
		if role == "" {
			role = "standalone"
		}
		health := nativeHealth(role, coordinator.InstanceID(), true)
		if opts.Deployment != "" {
			health.Deployment = opts.Deployment
		}
		localServer := &http.Server{
			Handler:           newLocalRouter(opts.Tracker, opts.PeerMgr, opts.PairingMgr, opts.PasskeyManager, health),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if err := localServer.Serve(unixListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.WithError(err).Error("unix socket listen error")
				serverErr <- err
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = localServer.Shutdown(shutdownCtx)
		}()
		logger.WithField("socket", socketPath).Info("listening on unix socket")
	}

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("unable to shutdown gracefully")
		return err
	}

	// Clean up socket file
	if unixListener != nil {
		_ = socket.Cleanup(socketPath)
	}

	return nil
}
