# GUPPI

all your tmux sessions, all your agents, one interface

get notified when it matters

---

## What is GUPPI?

guppi gives you a real-time web interface for your tmux sessions. It renders full terminal output in the browser using xterm.js backed by PTY connections, so you get the exact same view as your local terminal — borders, splits, colors, and all.

It also tracks AI coding agents (Claude Code, Codex, Copilot, OpenCode) running inside your sessions, surfacing their status so you know when an agent needs input, hits an error, or finishes a task.

### Key features

- **Full terminal in the browser** — PTY-backed xterm.js rendering. Type, scroll, resize — it just works.
- **Real-time session discovery** — sessions, windows, and panes update live via tmux control mode.
- **AI agent monitoring** — see which agents are active, waiting for input, or errored across all sessions at a glance.
- **Push notifications** — get browser/desktop notifications when an agent needs attention, even with the tab backgrounded.
- **Quick switcher** — Ctrl+K to jump between sessions and windows instantly, hands never leave the keyboard.
- **Single binary** — Go backend with the React frontend embedded. No separate processes, no Node runtime needed in production.
- **Unix socket + HTTP** — local CLI notifications go through a Unix socket for zero-config, with HTTP as fallback.
- **TLS first** -- TLS out of the gate, with easy instructions on trusting your CA certificate.

### Non-goals

- **Multi-user** — guppi is a single-user tool. One person, one dashboard. There are no user accounts, roles, or shared access controls.
- **Agent orchestration** — guppi doesn't start, stop, or control your agents. It watches and reports. You run your agents however you want; guppi just tells you what they're doing.
- **tmux management** — guppi doesn't configure or manage your tmux setup. Your `.tmux.conf`, layouts, and workflows stay yours.

## Installation

### Using dist (recommended)

[dist](https://github.com/ekristen/distillery) installs binaries from GitHub releases with checksum verification and multi-version support.

```bash
dist install ekristen/guppi
```

### Quick install

```bash
curl -sSL https://get.guppi.sh | sh
```

This detects your platform, downloads the latest release, and puts `guppi` in `/usr/local/bin` (or `~/.local/bin` if not writable). You can pin a version with `VERSION=v0.1.0 curl -sSL ... | sh`.

### From source

Requires [Go](https://go.dev/) 1.25+ and [Node.js](https://nodejs.org/) 18+.

```bash
git clone https://github.com/ekristen/guppi.git
cd guppi
make build
# Binary is at ./dist/guppi
```

## Usage

### 1. Start the server

Make sure [tmux](https://github.com/tmux/tmux) is running with at least one session, then:

```bash
guppi server
```

Open https://localhost:7654 in your browser. On first launch you'll set a password, then guppi will guide you through agent setup.

### 2. Configure agent hooks

guppi tracks AI agents running in your tmux sessions, but agents need hooks configured so they can report their status. Run:

```bash
guppi agent-setup
```

This auto-detects which agents you have installed and configures their hooks:

- **Claude Code** — hooks in `~/.claude/settings.json`
- **Codex** — `notify` command in `~/.codex/config.toml`
- **GitHub Copilot CLI** — hooks in `~/.copilot/hooks/guppi.json`
- **OpenCode** — plugin in `~/.config/opencode/plugins/guppi.js`

If you're running guppi in a multi-host setup, run `guppi agent-setup` on each machine where you use agents.

You can check hook status any time in the web UI under **Settings > Agents**, or by visiting `/setup`.

See [docs/agent-setup.md](docs/agent-setup.md) for manual setup instructions.

### 3. Use it

Once hooks are configured, agent status shows up automatically:

- The **Overview** page shows all sessions and any agents that need attention.
- The **sidebar** badges sessions with active/waiting/errored agents.
- **Push notifications** alert you when an agent needs input, even with the tab closed (enable in Settings > Notifications).
### Keyboard shortcuts

Press `Ctrl+/` (or `Cmd+/` on macOS) to see all shortcuts, or click the `?` in the status bar.

| Shortcut | Action |
|----------|--------|
| `Ctrl+K` | Quick switcher — jump between sessions and windows |
| `Ctrl+J` | Jump to next alert (cycles through waiting/error agents) |
| `Ctrl+H` | Overview |
| `Ctrl+,` | Settings |
| `Ctrl+\` | Toggle sidebar |
| `Ctrl+L` | Lock / sign out |
| `Ctrl+/` | Keyboard shortcuts help |

### Manual notifications

You can also send status updates from scripts or the command line:

```bash
guppi notify -t claude -s waiting -m "Needs approval"
guppi notify -t codex -s active
guppi notify -t claude -s completed
```

The tmux session, window, and pane are auto-detected when run inside tmux.

### Development

```bash
# Frontend dev server (hot reload)
cd web && npm install && npm run dev

# Go server (watches for tmux changes)
go run . server
```

## Architecture

```
Browser  <──WebSocket──>  Go Server  <──PTY──>  tmux attach-session
                              │
                              ├── Control mode (real-time state changes)
                              ├── Session discovery (polling fallback)
                              ├── Tool event tracker (agent status)
                              └── Unix socket (local CLI notifications)
```

Each browser tab gets its own PTY process running `tmux attach-session`. tmux handles all rendering natively — guppi just bridges the PTY output to xterm.js over a WebSocket. Window switching uses the tmux `select-window` command; tmux re-renders through the existing PTY connection.

State changes (new sessions, window renames, pane activity) are detected via tmux control mode and broadcast to all connected clients over a separate WebSocket.

## UI concepts

### Session status

Sessions in the sidebar and overview show as **active** or **idle**:

- **Active** — at least one pane in the session has a foreground process that isn't a shell. For example: `vim`, `claude`, `node`, `python`, `go build`, etc.
- **Idle** — every pane is sitting at a shell prompt (`bash`, `zsh`, `fish`, `sh`, `dash`, `ksh`, `csh`, `tcsh`, `tmux`, `login`).

This is driven by tmux's `pane_current_command`, which reports the foreground process of each pane. The server receives this via tmux control mode (or polling) and broadcasts it over WebSocket.

### Alerts

Alerts surface when an AI agent needs attention. They appear in the **alert banner** at the top of every page and in the **Pending Alerts** section on the overview.

- **Waiting** — the agent is waiting for user input (e.g., tool approval in Claude Code).
- **Error** — the agent hit an error.
- **Active** — the agent is running normally (shown as badges in the sidebar, not as alerts).

Alerts are live state from the server — they always reflect the current status and survive page refreshes. Dismissing an alert hides it from the UI but doesn't affect the agent.

Push alerts (via the Web Push API) work independently of the browser tab, including when logged out or when the tab is closed.

## Configuration

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GUPPI_PORT` | `7654` | HTTP server port |
| `GUPPI_SOCKET` | auto | Unix socket path for local CLI |
| `GUPPI_DISCOVERY_INTERVAL` | `2` | Session polling interval (seconds) |
| `GUPPI_NO_CONTROL_MODE` | `false` | Disable tmux control mode |
| `GUPPI_URL` | `https://localhost:7654` | Server URL for notify/agent-setup |
| `GUPPI_NO_AUTH` | `false` | Disable authentication |
| `GUPPI_NO_TLS` | `false` | Disable TLS (serve plain HTTP) |
| `GUPPI_TLS_CERT` | auto | Path to TLS certificate file |
| `GUPPI_TLS_KEY` | auto | Path to TLS private key file |
| `GUPPI_TLS_SAN` | | Additional TLS SANs (IPs or hostnames) |
| `GUPPI_HUB` | | Hub address for peer mode |
| `GUPPI_LOCAL_ONLY` | `false` | Only show local sessions in the web UI |
| `GUPPI_INSECURE` | `false` | Skip TLS verification when connecting to hub |
| `GUPPI_TLS_RELOAD_INTERVAL` | `60s` | Interval between TLS cert file change checks |

### CLI flags

```
guppi server [flags]
  -p, --port int                  HTTP server port (default 7654)
      --discovery-interval int    Session discovery interval in seconds (default 2)
      --no-control-mode           Disable tmux control mode (use polling only)
      --socket string             Unix socket path (auto-detected if omitted)
      --no-auth                   Disable authentication (not recommended for remote access)
      --no-tls                    Disable TLS (serve plain HTTP)
      --tls-cert string           Path to TLS certificate file (auto-generated if omitted)
      --tls-key string            Path to TLS private key file (auto-generated if omitted)
      --tls-san strings           Additional TLS SANs (IPs or hostnames, repeatable)
      --hub string                Hub address for peer mode (e.g. https://desktop.ts.net:7654)
      --local-only                Only show local sessions in the web UI
      --insecure                  Skip TLS verification when connecting to hub
      --tls-reload-interval duration  Interval between TLS cert file change checks (default 60s)
```

## FAQ

### How do I copy text from the terminal?

The terminal captures mouse events, so normal click-and-drag selects text inside tmux rather than copying to your clipboard. Hold a modifier key while selecting to override this and copy to the system clipboard:

| Platform | Select to copy |
|----------|---------------|
| **macOS** | Hold `Option` and drag to select, then `Cmd+C` to copy |
| **Linux** | Hold `Shift` and drag to select, then `Ctrl+Shift+C` to copy |
| **iOS (Safari)** | Touch-select doesn't work in the terminal. Connect a mouse or trackpad and use `Option`+drag, then copy from the context menu |

This is standard xterm.js behavior — the modifier key tells the browser to handle the selection instead of sending the mouse events to tmux.

## Tech stack

- **Backend:** Go, chi v5, gorilla/websocket, creack/pty
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS v4, xterm.js
- **Build:** Single binary with `//go:embed`, GoReleaser for releases

## License

MIT
