# CLAUDE.md

## Project Overview

TmuxAtlas is a web dashboard for monitoring and interacting with tmux sessions. Go backend + React/Vite frontend embedded in a single binary. The binary name is `tmuxatlas`.

## Quick Reference

| Task | Command |
|------|---------|
| Build everything | `/usr/bin/make build` |
| Build frontend only | `/usr/bin/make frontend` |
| Run dev server | `/usr/bin/make dev` |
| Run Go binary | `go run . server` |
| Frontend dev server | `cd web && npm run dev` |
| Run Go tests | `go test ./...` |
| Clean build artifacts | `/usr/bin/make clean` |

**Important:** Use `/usr/bin/make` (not `make`) due to a zsh shell function conflict.

## Architecture

- **PTY-based streaming:** Spawns `tmux attach-session` in a PTY per browser connection
- **WebSocket bridge:** Each browser tab gets its own PTY-to-WebSocket bridge
- **Embedded frontend:** Vite builds to `pkg/server/dist/`, Go embeds via `//go:embed`
- **Multi-host:** Star topology with Ed25519 peer identity over HTTP(S)/WebSocket transport

## Project Structure

```
main.go                  # Entry point, urfave/cli v3
pkg/
  commands/              # CLI commands (server, notify, pair, peers, agent-setup, install)
  server/                # Chi HTTP server + embedded frontend
  ws/                    # WebSocket hub (broadcast) + PTY terminal bridge
  state/                 # Central state tree with diff-based broadcasting
  tmux/                  # tmux client, PTY, control mode, discovery
  peer/                  # Multi-host peer protocol, manager, PTY relay
  identity/              # ed25519 keypair, peer store, pairing
  toolevents/            # Agent detection, event tracking, silence monitor, prompt parser
  auth/                  # Password auth + session management
  agentcheck/            # Agent installation & hook configuration checks
  activity/              # Activity tracking (sparklines)
  common/                # Command registration, version, logging setup
  socket/                # Unix socket communication
  preferences/           # User preferences
  stats/                 # System statistics
  webpush/               # Browser push notifications
web/
  src/
    App.tsx              # Main app component
    components/          # React components (Sidebar, Terminal, Overview, etc.)
    hooks/               # Custom hooks (useTerminal, useSessions, useHosts, etc.)
```

## Code Conventions

### Go
- **Module:** `github.com/LosFurina/tmuxatlas`
- **CLI framework:** urfave/cli v3
- **Command registration:** Commands register via `init()` calling `common.RegisterCommand()`, imported as blank imports in `main.go`
- **Logging:** logrus (`log.WithField(...)`)
- **HTTP router:** chi v5
- **WebSockets:** gorilla/websocket
- **Environment variables:** Prefixed with `TMUXATLAS_` (e.g., `TMUXATLAS_LISTEN`, `TMUXATLAS_PUBLIC_URL`, `TMUXATLAS_HUB`)
- **Testing:** Standard `testing` package, table-driven tests with `t.Run()` subtests

### Frontend
- **React 19** with TypeScript (strict mode)
- **Bundler:** Vite 6
- **Styling:** Tailwind CSS 4
- **Terminal:** xterm.js (`@xterm/xterm`)
- **State:** Custom React hooks (no Redux/Zustand)
- **Dev proxy:** Vite proxies to `http://localhost:7654`
- **Build output:** `../pkg/server/dist` (relative to `web/`)

### Multi-Host & Event Identity

**All tool events must include `Host` and `HostName` fields.** The frontend uses `host` to construct session keys (`host/session`) for URL routing and navigation. Events missing host info will fail to navigate to the correct session in multi-host setups.

- The HTTP handler (`POST /api/tool-event`) stamps `Host`/`HostName` from `PeerMgr.LocalID()`/`LocalName()` automatically.
- Components that record events directly (detector, silence monitor, inactivity promoter) must have host identity set via `SetHost()` at startup.
- Frontend session keys follow the format `host/sessionName` for remote sessions, or just `sessionName` for local-only (single host) mode.

### Agent Detection

Agent detection uses multiple layers — see `docs/agent-detection.md` for full details. Key rules:

- **Claude and OpenCode** have native "waiting" hooks. Codex and Copilot rely on silence-based detection or inactivity promotion.
- The silence monitor only runs `capture-pane` on panes with a detected non-Claude agent that have been silent for 10+ seconds, limited to 2 checks per silence period.
- The inactivity promoter (30s timeout) is a fallback for when silence detection doesn't trigger.
- The reconciler clears events when the agent process exits (foreground returns to a shell).

## Build & Release

- GoReleaser handles multi-platform builds (linux/darwin x amd64/arm64)
- The GoReleaser pre-hook runs `npm ci` and builds the embedded frontend before Go compilation
- Binaries are statically linked and stripped
- Version/commit injected via ldflags
- Pushing a semantic version tag such as `v0.2.0` or `v0.2.0-beta.1` runs CI, creates a draft GitHub Release, uploads signed checksums and SBOMs, and publishes the Release only after every job succeeds
