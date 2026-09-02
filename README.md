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
- **Command Palette** — search hosts, sessions, windows, agents, and application commands from one keyboard-first interface.
- **Single binary** — Go backend with the React frontend embedded. No separate processes, no Node runtime needed in production.
- **Unix socket + HTTP** — local CLI notifications go through a Unix socket for zero-config, with HTTP as fallback.
- **Gateway friendly** — serves a loopback HTTP origin designed for trusted TLS termination at Cloudflare Tunnel or Nginx.

### Non-goals

- **Multi-user** — TmuxAtlas is a single-user tool. One person, one dashboard. There are no user accounts, roles, or shared access controls.
- **Agent orchestration** — TmuxAtlas doesn't start, stop, or control your agents. It watches and reports. You run your agents however you want; TmuxAtlas just tells you what they're doing.
- **tmux management** — TmuxAtlas doesn't manage layouts or workflows. The installer can optionally add a small, clearly marked `mouse on` block to `.tmux.conf`; all other tmux configuration stays yours.

## Installation

### Choose the role for each machine

TmuxAtlas uses the same binary for four installation roles:

| Role | Use it when | What the installer starts |
|------|-------------|----------------------------|
| **Hub** | This machine hosts only the Web UI and aggregates remote Agent sessions | `tmuxatlas hub`, listening on loopback by default; no tmux dependency |
| **Standalone** | One machine hosts the Web UI and contributes its own tmux sessions | `tmuxatlas standalone` (`server` remains a compatibility alias) |
| **Agent** | This machine contributes its tmux sessions to an existing Hub | `tmuxatlas agent`, with an outbound-only WSS connection to the Hub |
| **Binary** | You want to configure or run TmuxAtlas manually | Nothing; only the executable is installed |

An Agent does **not** start a Web UI, HTTP listener, Passkey service, or public
endpoint. Do not run `tmuxatlas server --hub ...` on an Agent. The correct
long-running command is `tmuxatlas agent`, and the installer registers it as a
background user service.

The installer supports Linux (`systemd --user`) and macOS (`launchd`) on amd64
and arm64. It requires `curl`, `tar`, and either `sha256sum` or `shasum`;
Standalone and Agent machines also need `tmux`. A pure Hub does not.

### Interactive installation

Review [install.sh](install.sh), then run:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh | sh
```

The script asks which role this machine should use:

- **Hub** asks for the final browser-facing URL. Use the real HTTPS gateway
  hostname, such as `https://tmuxatlas.example.com`, before creating the first
  Passkey. For a local-only Hub, use `http://localhost:7654`.
- **Standalone** asks for the same public URL and also configures this machine's
  local tmux integration.
- **Agent** asks for the trusted Hub URL and a current six-word pairing code
  generated on the Hub.
- **Binary** skips role configuration and service creation.

The script downloads the newest GitHub Release, verifies its SHA-256 checksum,
and installs `tmuxatlas` to `~/.local/bin`. It can also add a clearly marked
`set -g mouse on` block to `~/.tmux.conf`. Role configuration is saved with
mode `0600` in `~/.config/tmuxatlas/.env`.

Override the defaults when needed:

```bash
TMUXATLAS_VERSION=v0.9.0 \
TMUXATLAS_INSTALL_DIR=/usr/local/bin \
sh install.sh
```

### 1. Install the Hub

Put a trusted HTTPS gateway such as Cloudflare Tunnel or Nginx+ACME in front of
the Hub. The default origin remains `127.0.0.1:7654`; the public URL is the
browser, Passkey, cookie, and Peer origin. Path-prefix hosting is not supported.

For an unattended Hub installation:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=hub \
  TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com \
  sh
```

This installs and immediately starts a pure `tmuxatlas hub` service:

| Platform | Service | Status and logs |
|----------|---------|-----------------|
| Linux | `tmuxatlas.service` | `systemctl --user status tmuxatlas.service` and `journalctl --user -u tmuxatlas.service -f` |
| macOS | `com.tmuxatlas.server` | `launchctl list com.tmuxatlas.server` and `tail -f ~/Library/Logs/com.tmuxatlas.server.stderr.log` |

On a headless Linux server, enable lingering for the installation user if the
Hub must survive SSH logout and start at boot:

```bash
sudo loginctl enable-linger "$USER"
```

Open the final `TMUXATLAS_PUBLIC_URL`. On first launch, retrieve the one-time
setup token from the service log, enter it in the browser, and enroll the
administrator Passkey. The browser can use the current device, a password
manager, hardware key, or an iPhone through its **Use another device** QR flow.

Verify the Hub before pairing another machine:

```bash
tmuxatlas doctor
```

See [Multi-host and trusted gateway deployment](docs/multi-host.md) for
Cloudflare Tunnel, Nginx+ACME, Passkey bootstrap, and reverse-proxy examples.
For a hardened container deployment, see [Docker Hub deployment](docs/docker.md).

### 2. Add an Agent

First generate a short-lived six-word pairing code on the running Hub:

```bash
tmuxatlas pair
```

Run the installer on the remote tmux machine, choose **Agent**, then enter the
Hub's final HTTPS URL and the generated code. For an unattended installation:

```sh
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=agent \
  TMUXATLAS_HUB=https://tmuxatlas.example.com \
  TMUXATLAS_PAIR_CODE=WORD-WORD-WORD-WORD-WORD-WORD \
  TMUXATLAS_CONFIGURE_TMUX=yes sh
```

The Agent installation performs the Ed25519 pairing handshake, saves the Hub
URL, and immediately starts the background service:

| Platform | Service | Status and logs |
|----------|---------|-----------------|
| Linux | `tmuxatlas-agent.service` | `systemctl --user status tmuxatlas-agent.service` and `journalctl --user -u tmuxatlas-agent.service -f` |
| macOS | `com.tmuxatlas.agent` | `launchctl list com.tmuxatlas.agent` and `tail -f ~/Library/Logs/com.tmuxatlas.agent.stderr.log` |

The same Linux lingering rule applies when an Agent must run without an active
login session.

The Agent opens only an outbound WSS connection to the trusted Hub and a
user-private Unix socket for local hooks. It does not need an inbound firewall
rule, public URL, Passkey, or TLS certificate. Cloudflare/ACME certificate
renewal does not require re-pairing because the Peer identity is independent
from the gateway certificate.

Configure AI-agent hooks on every machine where Claude Code, Codex, Copilot, or
OpenCode runs:

```bash
tmuxatlas agent-setup
tmuxatlas doctor
```

The new host and its tmux sessions should then appear in the Hub Web UI. If it
does not, check the Agent service log first.

### Binary-only and manual installation

Install only the verified executable, without writing `.env`, pairing, or
creating a service:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=binary \
  TMUXATLAS_CONFIGURE_TMUX=no sh
```

You can then configure and run either role manually:

```bash
# Manual pure Hub (no local tmux dependency)
TMUXATLAS_PUBLIC_URL=http://localhost:7654 tmuxatlas hub

# Manual standalone server (includes this machine's tmux sessions)
TMUXATLAS_PUBLIC_URL=http://localhost:7654 tmuxatlas standalone

# Manual Agent after running `tmuxatlas pair --hub ... --code ...`
TMUXATLAS_HUB=https://tmuxatlas.example.com tmuxatlas agent
```

For all unattended installations, set `TMUXATLAS_ROLE` explicitly. When the
script has no interactive terminal and no role is supplied, it deliberately
falls back to binary-only installation. Set `TMUXATLAS_CONFIGURE_TMUX=no` to
leave `.tmux.conf` untouched.

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

An immutable Docker deployment must be updated by pulling a new image and
recreating the container. Inside a container, `tmuxatlas update` and recovery
mutations fail closed; `tmuxatlas update --check` remains available. See
[Docker Hub deployment](docs/docker.md#update-and-rollback).

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

### 1. Start the server manually

The Hub installer already starts the server service. For a binary-only or
source installation, make sure [tmux](https://github.com/tmux/tmux) is running
with at least one session, then:

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

Press `Ctrl+/` (`Cmd+/` on macOS) to see the shortcuts generated from the
current command registry, or click the `?` in the status bar. In the table
below, **Mod** means `Ctrl` on Linux/Windows and `Cmd` on macOS.

| Shortcut | Action |
|----------|--------|
| `Mod+K` | Open the Command Palette (configurable to `Mod+P` or `Mod+Space` under **Settings → Interface**) |
| `Mod+/` | Open keyboard shortcuts help |
| `Mod+,` | Open Settings |
| `Mod+Shift+F` | Toggle Terminal fullscreen |
| `Mod+Shift+Z` | Toggle Terminal Zen Mode |

Overview, New Session, Reconnect Terminal, Toggle Sidebar, Go to Next Alert,
and Lock / Sign out are available as context-aware Command Palette actions.
Commands that cannot run in the current view are shown as unavailable.

Tmux control keys such as `Ctrl+H`, `Ctrl+J`, and `Ctrl+L` are deliberately not
application shortcuts. When the Terminal has focus, unregistered control keys
continue to the PTY unchanged.

### Command Palette

Open the Palette with `Mod+K`, or the alternative configured under
**Settings → Interface**. It searches and groups:

- **Hosts** by display name and stable Host ID.
- **Sessions** by Host, name, state, and attached agent information.
- **Windows** by Session, window name, and window index.
- **Agents** by agent name and its Host/Session target.
- **Commands** such as Overview, Settings, New Session, Reconnect, Fullscreen,
  Zen Mode, Sidebar, alerts, and sign out.

Pinned, recent, and needs-attention Sessions are promoted into their own
groups. Matching is fuzzy, so a few characters are normally enough. Use
`Up`/`Down` to move, `Enter` to open or run, and `Esc` to close; focus returns
to the control that opened the Palette. On narrow screens it uses a
Visual-Viewport-aware bottom sheet so the software keyboard does not push the
results off screen.

Targets with the same Session name on different Hosts remain separate. The
Host name and stable identity shown beside each result indicate which machine
will receive the navigation action.

### Terminal Search and Zen Mode

Choose **Find** in the Terminal Cockpit, **Find in Terminal** from the Terminal
context menu, or the equivalent Cockpit action to search the current
scrollback. Search supports live match counts, case-sensitive matching,
previous/next navigation, and these keys:

| Search key | Action |
|------------|--------|
| `Enter` | Next match |
| `Shift+Enter` | Previous match |
| `Esc` | Close Search and return focus to the Terminal |

Search belongs to the current Terminal target. Switching Host or Session
clears its query/results, and a Search module that fails to load offers
**Retry Search** rather than consuming Terminal input.

Zen Mode is intended for focused Terminal work. Enter it from the Command
Palette or with `Mod+Shift+Z`; it hides the Sidebar, Top Bar, Status Bar, and
alert chrome without replacing the active PTY connection. Use the floating
**Exit Zen** control or the same shortcut to leave it. Fullscreen
(`Mod+Shift+F`) is independent and can be combined with Zen Mode.

### Clipboard and Terminal context menu

The Terminal Cockpit exposes selection-aware **Copy** and connection-aware
**Paste** actions. Right-click the Terminal, or open **More Terminal actions**,
for **Copy selection**, **Paste**, **Find in Terminal**, and **Select all**.
Copy sends only the current xterm selection to the browser clipboard.

Clipboard Paste is always initiated by a user gesture:

- A single-line value is sent immediately when the browser grants clipboard
  access.
- A value containing `LF` or `CR` opens a confirmation dialog that shows the
  exact target, line count, and text before anything is sent.
- Confirmed Paste follows the active Terminal's bracketed-paste mode.
- If clipboard access is unavailable or denied, the clipboard is empty, the
  target changes, or the WebSocket generation becomes stale, no paste bytes
  are sent and the UI reports the error.

### Mobile Input Composer

On narrow screens, expand **Input Composer** below the Terminal to prepare a
command in a one-to-three-line text area. **Send** behaves like typing the
entire text and then pressing Enter, with a strict wire contract:

```text
one Binary WebSocket frame = UTF8(exact textarea value) + CR (0x0D)
```

TmuxAtlas does not trim, parse, shell-escape, interpolate, or normalize the
textarea body. Leading/trailing spaces, quotes, shell metacharacters,
newlines, Chinese text, and emoji therefore arrive as entered. An empty body
is valid and sends one `CR`, equivalent to pressing Enter at the current
prompt. The body limit is **65,535 UTF-8 bytes** (the final `CR` is separate);
an oversized draft is retained and nothing is sent. The byte counter is more
important than character count for Unicode input.

Ordinary `Enter` inserts a newline. Use the **Send** button or
`Ctrl+Enter`/`Cmd+Enter` to submit; an active IME composition is never
submitted by that shortcut. Composer submissions bypass the mobile
Ctrl/Alt modifier encoder, so a selected modifier cannot rewrite the composed
text.

Drafts exist only in page memory and are isolated by stable
`Host ID + Session` target. They are never written to browser storage or the
server and are never replayed automatically after a disconnect, reconnect, or
target change. Sending captures both the target and WebSocket generation: a
closed or stale connection keeps the draft visible and sends no frame. A
successful send clears only that target's draft.

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
| `TMUXATLAS_DISCOVERY_INTERVAL` | `2` | Session polling interval (seconds) |
| `TMUXATLAS_NO_CONTROL_MODE` | `false` | Disable tmux control mode |
| `TMUXATLAS_SOCKET` | platform default | Private Unix socket used by notify/agent-setup |
| `TMUXATLAS_DEPLOYMENT` | `native` | Runtime packaging; the official container sets `docker` |
| `TMUXATLAS_NO_AUTH` | `false` | Disable application authentication; ingress scope is an operator responsibility |
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
      --no-auth                   Disable application authentication (operator-selected ingress)
      --hub string                Trusted hub URL for peer mode (e.g. https://tmuxatlas.example.com)
      --local-only                Only show local sessions in the web UI
```

Use `tmuxatlas hub` for a remote-only Hub and `tmuxatlas standalone` when the
same process should also expose local tmux sessions. `tmuxatlas server` remains
an alias for standalone mode.

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
| **macOS** | Hold `Option` and drag, then use Cockpit **Copy**, context-menu **Copy selection**, or `Cmd+C` |
| **Linux** | Hold `Shift` and drag, then use Cockpit **Copy**, context-menu **Copy selection**, or `Ctrl+C` while the selection is active |
| **iOS (Safari)** | Touch selection is limited in xterm. Connect a mouse or trackpad, select with `Option`+drag, then use **Copy selection** |

The selection modifier tells xterm.js not to send that mouse gesture to tmux.
Copy is disabled when there is no selection, so an accidental Copy action
cannot read unrelated Terminal content.

## Tech stack

- **Backend:** Go, chi v5, gorilla/websocket, creack/pty
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS v4, xterm.js
- **Build:** Single binary with `//go:embed`, GoReleaser for releases

## License

MIT
