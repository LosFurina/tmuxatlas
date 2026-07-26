# TmuxAtlas

all your tmux sessions, all your agents, one interface

get notified when it matters

---

## What is TmuxAtlas?

TmuxAtlas gives you a real-time web interface for your tmux sessions. It renders full terminal output in the browser using xterm.js backed by PTY connections, so you get the exact same view as your local terminal — borders, splits, colors, and all.

It also tracks AI coding agents (Claude Code, Codex, Copilot, OpenCode) running inside your sessions, surfacing their status so you know when an agent needs input, hits an error, or finishes a task.

### Key features

- **Full terminal in the browser** — PTY-backed xterm.js rendering. Type, scroll, resize — it just works.
- **Real-time session discovery** — sessions, windows, and panes update live via tmux control mode.
- **AI agent monitoring** — see which agents are active, waiting for input, or errored across all sessions at a glance.
- **Push notifications** — get browser/desktop notifications when an agent needs attention, even with the tab backgrounded.
- **Installable Web App** — install the HTTPS Hub as a focused PWA on desktop or add it to an iPhone/iPad Home Screen.
- **Quick switcher** — Ctrl+K to jump between sessions and windows instantly, hands never leave the keyboard.
- **Single binary** — Go backend with the React frontend embedded. No separate processes, no Node runtime needed in production.
- **Unix socket + HTTP** — local CLI notifications go through a Unix socket for zero-config, with HTTP as fallback.
- **Gateway friendly** — serves a loopback HTTP origin designed for trusted TLS termination at Cloudflare Tunnel or Nginx.

### Non-goals

- **Multi-user** — TmuxAtlas is a single-user tool. One person, one dashboard. There are no user accounts, roles, or shared access controls.
- **Agent orchestration** — TmuxAtlas doesn't start, stop, or control your agents. It watches and reports. You run your agents however you want; TmuxAtlas just tells you what they're doing.
- **tmux management** — TmuxAtlas doesn't manage layouts or workflows. The installer can optionally add a small, clearly marked `mouse on` block to `.tmux.conf`; all other tmux configuration stays yours.

## Installation

### Install the latest release

Review [install.sh](install.sh), then run:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh | sh
```

The script first asks whether this machine is a Hub, an outbound-only Agent, or
a binary-only installation. It then asks only for the URL relevant to that
role, downloads the newest GitHub Release, verifies its SHA-256 checksum, and
installs `tmuxatlas` to `~/.local/bin`. Hub and Agent roles are installed as a
systemd/launchd user service; Agent installation also completes pairing.

Override the defaults when needed:

```bash
TMUXATLAS_VERSION=v0.7.0 \
TMUXATLAS_INSTALL_DIR=/usr/local/bin \
sh install.sh
```

For unattended installs, choose explicitly instead of waiting for a prompt:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=hub \
  TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com \
  TMUXATLAS_CONFIGURE_TMUX=yes sh
```

For an unattended Agent installation:

```sh
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=agent \
  TMUXATLAS_HUB=https://tmuxatlas.example.com \
  TMUXATLAS_PAIR_CODE=WORD-WORD-WORD sh
```

Set `TMUXATLAS_ROLE=binary` for no configuration or service. Set
`TMUXATLAS_CONFIGURE_TMUX=no` to leave `.tmux.conf` untouched.

### Update and diagnose

Update an installed binary from the latest GitHub Release:

```bash
tmuxatlas update
```

The updater downloads the release archive and `checksums.txt`, verifies the
SHA-256 checksum, and atomically replaces the currently running executable. It
does not modify configuration, Passkeys, peer identities, or other user data.
When the installed systemd/launchd service points to the same executable and
was already running, the updater restarts it automatically. An inactive service
is not started. The update is rejected if the service points to a different
binary, preventing the wrong copy from being replaced.

Use `tmuxatlas update --check` to check without installing,
`--no-restart` to defer a service restart, or `--force` to reinstall the current
version. Restarting clears in-memory browser sessions, so the next visit
requires Passkey login.

No token is needed for the public repository. If GitHub API rate limiting is a
problem, set `GITHUB_TOKEN` or `GH_TOKEN` in the process environment.

Inspect a local installation with:

```bash
tmuxatlas doctor
```

Doctor checks the executable, tmux, `.env`, public Passkey origin, session TTL,
Passkey store and permissions, listening server, legacy password file, and
systemd/launchd user service. Warnings are informational; failed checks produce
a non-zero exit status.

### Download a release

Download the appropriate archive for your platform from the
[LosFurina/tmuxatlas releases](https://github.com/LosFurina/tmuxatlas/releases) page.

### From source

Requires [Go](https://go.dev/) 1.25+ and [Node.js](https://nodejs.org/) 18+.

```bash
git clone https://github.com/LosFurina/tmuxatlas.git
cd tmuxatlas
make build
# Binary is at ./dist/tmuxatlas
```

## Usage

### 1. Start the server

Make sure [tmux](https://github.com/tmux/tmux) is running with at least one session, then:

```bash
tmuxatlas server
```

The server prints a one-time setup token on first launch. Open
http://localhost:7654, enter that token, and create the administrator passkey.
The browser can use the current device, a passkey manager such as Proton Pass,
Bitwarden, or 1Password, or another device such as an iPhone by displaying a QR
code.

For remote access, keep TmuxAtlas on loopback and put a trusted HTTPS gateway in front:

```bash
TMUXATLAS_LISTEN=127.0.0.1:7654 \
TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com \
tmuxatlas server
```

Set `TMUXATLAS_PUBLIC_URL` to the final browser-facing URL **before creating the
first passkey**. WebAuthn binds passkeys to that hostname, and HTTPS is required
except for the browser's localhost development exception. The setting also
gives authentication cookies the `Secure` attribute. See
[Multi-host and trusted gateway deployment](docs/multi-host.md) for Cloudflare
Tunnel and Nginx+ACME examples.

TmuxAtlas does not store a password or a passkey private key. It stores only the
public WebAuthn credential record in `~/.config/tmuxatlas/passkeys.json` with
mode `0600`. Upgrades from password-based releases ignore the old `auth.json`;
after verifying Passkey login, you may delete that legacy file manually.

After signing in, open **Settings → Security → Passkeys** to add a backup
Passkey, rename credentials, or remove one you no longer control. Provider
selection belongs to the browser: use a device Passkey, scan the browser's QR
code with an iPhone, or choose Proton Pass, Bitwarden, 1Password, or another
WebAuthn-compatible provider. Add and verify a backup before deleting anything;
TmuxAtlas refuses to delete the final registered Passkey.

There is no in-app recovery when every authenticator is lost. An operator with
shell access can stop TmuxAtlas, move `~/.config/tmuxatlas/passkeys.json` aside,
and restart to bootstrap a new administrator Passkey, but that reset invalidates
every previously registered credential. Backing up `passkeys.json` alone cannot
replace the private keys held by your devices or password manager.

### 2. Configure agent hooks

TmuxAtlas tracks AI agents running in your tmux sessions, but agents need hooks configured so they can report their status. Run:

```bash
tmuxatlas agent-setup
```

This auto-detects which agents you have installed and configures their hooks:

- **Claude Code** — hooks in `~/.claude/settings.json`
- **Codex** — `notify` command in `~/.codex/config.toml`
- **GitHub Copilot CLI** — hooks in `~/.copilot/hooks/tmuxatlas.json`
- **OpenCode** — plugin in `~/.config/opencode/plugins/tmuxatlas.js`

If you're running TmuxAtlas in a multi-host setup, run `tmuxatlas agent-setup` on each machine where you use agents.

You can check hook status any time in the web UI under **Settings > Agents**, or by visiting `/setup`.

See [docs/agent-setup.md](docs/agent-setup.md) for manual setup instructions.

### 3. Use it

Once hooks are configured, agent status shows up automatically:

- The **Overview** page shows all sessions and any agents that need attention.
- The **sidebar** badges sessions with active/waiting/errored agents.
- **Push notifications** alert you when an agent needs input, even with the tab closed (enable in Settings > Notifications).

### Install as a Web App

TmuxAtlas can be installed from the same HTTPS hostname used for Passkey login:

- In Chrome, Edge, or another Chromium browser, open **Settings → Interface →
  Install TmuxAtlas** when the browser offers the install action. The browser's
  address-bar or application menu can expose the same native action.
- On iPhone or iPad, open the Hub in Safari, tap **Share**, then choose
  **Add to Home Screen**. TmuxAtlas also shows these instructions under
  **Settings → Interface**.

The installed app remains on the Hub's existing origin, so it uses the same
Passkey RP ID, Secure cookie, APIs, WebSockets, and Push subscription as the
ordinary browser tab. Existing Passkeys do not need to be enrolled again.
Cloudflare Tunnel or Nginx+ACME works without a separate application origin or
Cloudflare Access; install from the final `TMUXATLAS_PUBLIC_URL` hostname.

TmuxAtlas is online-only. Its Service Worker handles Push notifications but
does not cache the application shell, authentication state, API responses, or
terminal traffic. When the Hub or gateway is unreachable, the installed app
shows the normal disconnected state rather than stale terminal content.

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
tmuxatlas notify -t claude -s waiting -m "Needs approval"
tmuxatlas notify -t codex -s active
tmuxatlas notify -t claude -s completed
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

Each browser tab gets its own PTY process running `tmux attach-session`. tmux handles all rendering natively — TmuxAtlas just bridges the PTY output to xterm.js over a WebSocket. Window switching uses the tmux `select-window` command; tmux re-renders through the existing PTY connection.

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

TmuxAtlas automatically loads `~/.config/tmuxatlas/.env` at startup. Existing
process environment variables take precedence. Start from
[`.env.example`](.env.example); only `TMUXATLAS_*` entries are accepted.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TMUXATLAS_LISTEN` | `127.0.0.1:7654` | HTTP/WS origin listen address |
| `TMUXATLAS_PUBLIC_URL` | `http://localhost:7654` | Browser-facing absolute HTTP(S) URL; HTTPS enables Secure cookies |
| `TMUXATLAS_SESSION_TTL` | `24h` | Idle time before Passkey login is required again; accepts Go durations such as `168h` |
| `TMUXATLAS_SOCKET` | auto | Unix socket path for local CLI |
| `TMUXATLAS_DISCOVERY_INTERVAL` | `2` | Session polling interval (seconds) |
| `TMUXATLAS_NO_CONTROL_MODE` | `false` | Disable tmux control mode |
| `TMUXATLAS_SOCKET` | platform default | Private Unix socket used by notify/agent-setup |
| `TMUXATLAS_NO_AUTH` | `false` | Disable authentication |
| `TMUXATLAS_HUB` | | Hub URL for peer mode; use the gateway's trusted HTTPS URL |
| `TMUXATLAS_LOCAL_ONLY` | `false` | Only show local sessions in the web UI |
| `TMUXATLAS_PEER_OUTCOME_TTL` | `5m` | Agent-side correlated action result retention |
| `TMUXATLAS_PEER_OUTCOME_MAX_ENTRIES` | `1024` | Maximum retained/in-flight correlated Agent actions |
| `TMUXATLAS_PEER_OUTCOME_MAX_BYTES` | `65536` | Maximum serialized result/error bytes per action |

### CLI flags

```
tmuxatlas server [flags]
      --listen string             HTTP/WS origin listen address (default "127.0.0.1:7654")
      --public-url string         Browser-facing absolute URL (default "http://localhost:7654")
      --session-ttl duration      Idle time before Passkey login is required again (default 24h)
      --discovery-interval int    Session discovery interval in seconds (default 2)
      --no-control-mode           Disable tmux control mode (use polling only)
      --socket string             Unix socket path (auto-detected if omitted)
      --no-auth                   Disable authentication (loopback development only)
      --hub string                Trusted hub URL for peer mode (e.g. https://tmuxatlas.example.com)
      --local-only                Only show local sessions in the web UI
```

### Upgrading from guppi

TmuxAtlas automatically copies existing configuration from `~/.config/guppi` to `~/.config/tmuxatlas` and migrates application data such as Web Push keys. The original directories are retained for rollback and existing files in the new directory are never overwritten.

The old `GUPPI_*` runtime variables remain accepted as deprecated aliases, but new deployments should use `TMUXATLAS_*`. After installing the renamed binary:

```bash
tmuxatlas install --public-url https://tmuxatlas.example.com
tmuxatlas agent-setup
```

The first command installs the new `tmuxatlas.service` or `com.tmuxatlas.server` service and stops the old service while retaining its definition. The second rewrites agent commands and removes legacy Copilot/OpenCode hook files so events are not sent twice.

### Upgrading from built-in TLS

Built-in TLS and private certificate trust have been removed. Before upgrading a remote deployment, configure a trusted gateway and remove `--port`, `--no-tls`, `--tls-cert`, `--tls-key`, `--tls-san`, `--tls-reload-interval`, `--insecure`, and their corresponding environment variables. Use `--listen` and `--public-url` instead.

Legacy peer records are migrated automatically: TmuxAtlas preserves each peer's name, Ed25519 public key, and pairing time, removes only obsolete CA/leaf-certificate fields, and writes a one-time `peers.json.pre-system-trust.bak` rollback copy. Existing certificate files are deliberately left untouched. After verifying the new gateway deployment, you may manually delete unused certificate and key files from the TmuxAtlas configuration directory.

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
