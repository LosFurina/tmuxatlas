package server

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/auth"
	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/identity"
	"github.com/LosFurina/tmuxatlas/pkg/peer"
	"github.com/LosFurina/tmuxatlas/pkg/preferences"
	"github.com/LosFurina/tmuxatlas/pkg/server"
	"github.com/LosFurina/tmuxatlas/pkg/state"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
	"github.com/LosFurina/tmuxatlas/pkg/webpush"
)

const defaultListenAddress = "127.0.0.1:7654"

var removedTransportEnvVars = []string{
	"TMUXATLAS_PORT",
	"TMUXATLAS_NO_TLS",
	"TMUXATLAS_TLS_CERT",
	"TMUXATLAS_TLS_KEY",
	"TMUXATLAS_TLS_SAN",
	"TMUXATLAS_TLS_RELOAD_INTERVAL",
	"TMUXATLAS_INSECURE",
	"GUPPI_PORT",
	"GUPPI_NO_TLS",
	"GUPPI_TLS_CERT",
	"GUPPI_TLS_KEY",
	"GUPPI_TLS_SAN",
	"GUPPI_TLS_RELOAD_INTERVAL",
	"GUPPI_INSECURE",
}

func validateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if host == "" {
		return fmt.Errorf("invalid listen address %q: host is required", address)
	}
	if port == "" {
		return fmt.Errorf("invalid listen address %q: port is required", address)
	}
	return nil
}

func validatePublicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid public URL %q: %w", raw, err)
	}
	if !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("public URL must be an absolute http or https URL")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("public URL must not contain credentials, a path, query, or fragment")
	}
	return u, nil
}

func validateRemovedTransportEnv() error {
	for _, name := range removedTransportEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			return fmt.Errorf("%s is no longer supported; terminate TLS at a trusted gateway and use TMUXATLAS_LISTEN/TMUXATLAS_PUBLIC_URL", name)
		}
	}
	return nil
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}

func Execute(ctx context.Context, c *cli.Command) error {
	// Initialize tmux client
	client, err := tmux.NewClient()
	if err != nil {
		return err
	}

	// Initialize state manager
	stateMgr := state.NewManager(client)

	// Initialize tool event tracker
	tracker := toolevents.NewTracker()

	// Initialize activity tracker
	actTracker := activity.NewTracker()

	// Start session discovery in background
	interval := time.Duration(c.Int("discovery-interval")) * time.Second
	discovery := tmux.NewDiscovery(client, interval, func(sessions []*tmux.Session) {
		stateMgr.UpdateSessions(sessions)
	})
	go discovery.Run(ctx)

	// Start tool event reconciler — clears stale notifications when the
	// agent process is no longer running in the pane
	reconciler := toolevents.NewReconciler(tracker, func(paneID string) toolevents.PaneState {
		panes, err := client.ListAllPanes()
		if err != nil {
			return toolevents.PaneState{Exists: false}
		}
		for _, p := range panes {
			if p.ID == paneID {
				return toolevents.PaneState{Exists: true, CurrentCommand: p.CurrentCommand, PID: p.PID}
			}
		}
		return toolevents.PaneState{Exists: false}
	}, 3*time.Second)
	go reconciler.Run(ctx)

	// Start agent detector — scans pane process trees for known agents
	// and records synthetic events for panes without hook-based tracking
	detector := toolevents.NewDetector(tracker, func() []toolevents.PaneInfo {
		panes, err := client.ListAllPanesDetailed()
		if err != nil {
			return nil
		}
		var infos []toolevents.PaneInfo
		for _, p := range panes {
			infos = append(infos, toolevents.PaneInfo{
				PaneID:  p.ID,
				Session: p.Session,
				Window:  p.Window,
				PID:     p.PID,
			})
		}
		return infos
	}, 5*time.Second)
	go detector.Run(ctx)

	// Start silence monitor — detects when non-Claude agents go quiet and
	// inspects pane content for input prompts
	silenceMonitor := toolevents.NewSilenceMonitor(tracker, detector, client)
	go silenceMonitor.Run(ctx)

	// Start control mode for event-driven state updates
	if !c.Bool("no-control-mode") {
		fallbackInterval := 30 * time.Second
		ctrlMode := tmux.NewControlMode(client, func(sessions []*tmux.Session) {
			stateMgr.UpdateSessions(sessions)
		},
			tmux.WithOnConnect(func() {
				discovery.SetInterval(fallbackInterval)
			}),
			tmux.WithOnDisconnect(func() {
				discovery.SetInterval(interval)
			}),
			tmux.WithOnOutput(func(paneID string, dataLen int) {
				session := stateMgr.SessionForPane(paneID)
				if session != "" {
					actTracker.Record(session, dataLen)
				}
				silenceMonitor.RecordOutput(paneID)
			}),
		)
		go ctrlMode.Run(ctx)
	}

	// Start inactivity promoter — generates synthetic "waiting" events for
	// tools that lack native waiting detection (copilot, codex, opencode)
	go tracker.RunInactivityPromoter(ctx, toolevents.DefaultInactivityTimeout)

	// Initialize preferences store
	prefStore, err := preferences.NewStore()
	if err != nil {
		logrus.WithError(err).Warn("failed to load preferences, using defaults")
		// Create a fallback in-memory store with defaults
		prefStore = nil
	}

	// Initialize web push notifications
	var pushKeys *webpush.VAPIDKeys
	var pushStore *webpush.Store
	vapidKeys, err := webpush.LoadOrCreateKeys()
	if err != nil {
		logrus.WithError(err).Warn("failed to load VAPID keys, push notifications will be unavailable")
	} else {
		pushKeys = vapidKeys
		pushStore = webpush.NewStore()
		pushSender := webpush.NewSender(pushKeys, pushStore, tracker)
		go pushSender.Run(ctx)
	}

	publicURL, err := validatePublicURL(c.String("public-url"))
	if err != nil {
		return err
	}
	listenAddress := c.String("listen")
	if err := validateListenAddress(listenAddress); err != nil {
		return err
	}

	// Initialize Passkey-only authentication. The RP ID and allowed origin are
	// derived from the externally reachable URL, not the internal listen address.
	var (
		authEnabled    bool
		passkeyManager *auth.PasskeyManager
		sessionMgr     *auth.SessionManager
	)
	if !c.Bool("no-auth") {
		sessionMgr = auth.NewSessionManager(24 * time.Hour)
		passkeyManager, err = auth.NewPasskeyManager(publicURL.String(), sessionMgr)
		if err != nil {
			return fmt.Errorf("failed to initialize auth: %w", err)
		}
		authEnabled = true

		if !passkeyManager.HasCredentials() {
			logrus.WithFields(logrus.Fields{
				"setup_token": passkeyManager.BootstrapToken(),
				"origin":      passkeyManager.Origin(),
			}).Warn("PASSKEY SETUP REQUIRED — open the public URL and enter this one-time setup token")
		}
	}

	// Initialize identity for peer system
	hostname, _ := os.Hostname()
	nodeIdentity, err := identity.LoadOrCreate(hostname)
	if err != nil {
		return fmt.Errorf("failed to load identity: %w", err)
	}
	logrus.WithField("name", nodeIdentity.Name).WithField("fingerprint", nodeIdentity.Fingerprint()).Info("node identity loaded")

	peerStore, err := identity.NewPeerStore()
	if err != nil {
		return fmt.Errorf("failed to load peer store: %w", err)
	}

	// Initialize peer manager
	peerMgr := peer.NewManager(nodeIdentity, peerStore, stateMgr)
	go peerMgr.Run()

	// Stamp local host identity on detector and silence monitor so
	// auto-detected events include the host info for multi-host navigation
	detector.SetHost(peerMgr.LocalID(), peerMgr.LocalName())
	silenceMonitor.SetHost(peerMgr.LocalID(), peerMgr.LocalName())

	// Initialize pairing manager
	pairingMgr := identity.NewPairingManager()

	// Initialize PTY relay for remote sessions
	ptyRelay := peer.NewPTYRelay()

	// Initialize peer handler (accepts incoming peer connections)
	peerHandler := peer.NewHandler(peerMgr, peerStore, tracker, pairingMgr, ptyRelay)

	// If --hub is set, connect to the hub as a peer
	hubURL := c.String("hub")
	localOnly := c.Bool("local-only")
	if hubURL != "" {
		peerClient := peer.NewClient(
			hubURL, nodeIdentity, peerStore,
			stateMgr, peerMgr, actTracker, tracker,
			client.TmuxPath(),
		)
		go peerClient.Run(ctx)
		logrus.WithField("hub", hubURL).Info("connecting to hub as peer")
	}

	if !isLoopbackListen(listenAddress) {
		logrus.WithField("listen", listenAddress).
			Warn("TmuxAtlas origin is using plaintext HTTP on a non-loopback address; protect it with a trusted private network")
	}

	opts := &server.Options{
		ListenAddress:   listenAddress,
		PublicURL:       publicURL.String(),
		SecureCookies:   publicURL.Scheme == "https",
		SocketPath:      c.String("socket"),
		Client:          client,
		StateMgr:        stateMgr,
		Tracker:         tracker,
		ActivityTracker: actTracker,
		PushKeys:        pushKeys,
		PushStore:       pushStore,
		PrefStore:       prefStore,
		AuthEnabled:     authEnabled,
		PasskeyManager:  passkeyManager,
		SessionMgr:      sessionMgr,
		PeerMgr:         peerMgr,
		PeerHandler:     peerHandler,
		PairingMgr:      pairingMgr,
		PTYRelay:        ptyRelay,
		Detector:        detector,
		LocalOnly:       localOnly,
	}

	// Start the HTTP server (blocks until ctx is cancelled)
	return server.Run(ctx, opts)
}

func serverFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "HTTP origin listen address",
			Sources: cli.EnvVars("TMUXATLAS_LISTEN", "GUPPI_LISTEN"),
			Value:   defaultListenAddress,
		},
		&cli.StringFlag{
			Name:    "public-url",
			Usage:   "Externally reachable HTTP(S) URL; HTTPS enables Secure cookies",
			Sources: cli.EnvVars("TMUXATLAS_PUBLIC_URL", "GUPPI_PUBLIC_URL"),
			Value:   "http://localhost:7654",
		},
		&cli.IntFlag{
			Name:    "discovery-interval",
			Usage:   "Session discovery interval in seconds",
			Sources: cli.EnvVars("TMUXATLAS_DISCOVERY_INTERVAL", "GUPPI_DISCOVERY_INTERVAL"),
			Value:   2,
		},
		&cli.BoolFlag{
			Name:    "no-control-mode",
			Usage:   "Disable tmux control mode (use polling only)",
			Sources: cli.EnvVars("TMUXATLAS_NO_CONTROL_MODE", "GUPPI_NO_CONTROL_MODE"),
		},
		&cli.StringFlag{
			Name:    "socket",
			Usage:   "Unix socket path for local notify CLI (auto-detected if omitted)",
			Sources: cli.EnvVars("TMUXATLAS_SOCKET", "GUPPI_SOCKET"),
		},
		&cli.BoolFlag{
			Name:    "no-auth",
			Usage:   "Disable authentication (not recommended for remote access)",
			Sources: cli.EnvVars("TMUXATLAS_NO_AUTH", "GUPPI_NO_AUTH"),
		},
		&cli.StringFlag{
			Name:    "hub",
			Usage:   "Trusted hub URL to connect to as a peer (e.g. https://tmuxatlas.example.com)",
			Sources: cli.EnvVars("TMUXATLAS_HUB", "GUPPI_HUB"),
		},
		&cli.BoolFlag{
			Name:    "local-only",
			Usage:   "Only show local sessions in the web UI (still shares state with hub)",
			Sources: cli.EnvVars("TMUXATLAS_LOCAL_ONLY", "GUPPI_LOCAL_ONLY"),
		},
	}
}

func init() {
	cmd := &cli.Command{
		Name:        "server",
		Usage:       "start the TmuxAtlas web server",
		Description: "starts the web dashboard for monitoring and interacting with tmux sessions",
		Flags:       serverFlags(),
		Action:      Execute,
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			if err := validateRemovedTransportEnv(); err != nil {
				return ctx, err
			}
			if err := validateListenAddress(c.String("listen")); err != nil {
				return ctx, err
			}
			if _, err := validatePublicURL(c.String("public-url")); err != nil {
				return ctx, err
			}
			logrus.Info("checking for tmux...")
			_, err := tmux.NewClient()
			if err != nil {
				return ctx, err
			}
			logrus.Info("tmux found")
			return ctx, nil
		},
	}

	common.RegisterCommand(cmd)
}
