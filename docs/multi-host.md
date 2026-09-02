# Multi-Host and Trusted Gateway Deployment

TmuxAtlas can aggregate tmux sessions from several machines into one dashboard. The hub and peers use an application-level Ed25519 identity; public TLS belongs to a trusted gateway such as Cloudflare Tunnel or Nginx with an ACME certificate. Gateway filtering is additional defense: when enabled, TmuxAtlas enforces Passkey authentication, exact Host/Origin checks, resource limits, and Peer proofs.

`--no-auth` is an operator-controlled deployment choice and is accepted with any valid listener and Public URL. It disables TmuxAtlas application authentication and public Host/Origin gating for every reachable client, allowing a single instance to serve both loopback and LAN origins. Use it only when the operator intentionally accepts that exposure and has provided an appropriate network boundary. It can be used behind Cloudflare Tunnel, Nginx, or another gateway when that is the operator's chosen trust model.

Default application ingress limits are finite: 32 KiB request headers, 1 MiB
global bodies, 4 KiB ordinary JSON, 16 KiB pairing JSON, 128 KiB
WebAuthn/Push JSON, 5-second header reads, and 10–15-second JSON body reads.
WebSocket categories each permit at most 64 live connections, with independent
rate buckets for browser terminal, Peer control, and Peer PTY traffic. WebAuthn
allows 16 and pairing 12 concurrent operations; the application-wide in-flight
ceiling is 128. These defaults protect a single-admin Hub and remain enforced
behind Cloudflare Tunnel or Nginx.

## Architecture and trust boundaries

```text
Browser ── HTTPS/WSS ──┐
                       ▼
Peer ───── HTTPS/WSS ─ Gateway ── HTTP/WS on loopback ── tmuxatlas hub
                                                        ▲
Peer ───── HTTPS/WSS ───────────────────────────────────┘
```

- The gateway authenticates the public hostname with a system-trusted certificate and forwards to `127.0.0.1:7654`.
- TmuxAtlas Passkey authentication protects browser access. User verification
  is required for every registration and login.
- Pairing stores Ed25519 public keys. Each peer control connection must sign a fresh challenge, independently of the gateway certificate.
- `/ws/peer` carries long-lived state synchronization. `/ws/peer-pty?stream=...` carries remote terminal streams.

Use a dedicated hostname such as `tmuxatlas.example.com`; path-prefix hosting is not supported. The gateway must preserve the public `Host`, retain query strings, and support WebSocket upgrades. TmuxAtlas does not require or integrate with Cloudflare Access.

## Start the hub

Keep the origin on loopback and describe the browser-facing URL explicitly:

```bash
TMUXATLAS_LISTEN=127.0.0.1:7654 \
TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com \
tmuxatlas server
```

The one-line installer asks for this URL and stores it in
`~/.config/tmuxatlas/.env`. For unattended installation, set it explicitly:

```bash
curl -fsSL https://raw.githubusercontent.com/LosFurina/tmuxatlas/main/install.sh |
  TMUXATLAS_ROLE=hub \
  TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com sh
```

`TMUXATLAS_PUBLIC_URL` is also the WebAuthn origin and determines the Passkey
relying-party ID. Set it to the final public hostname before enrollment; a
Passkey created for `localhost` cannot sign in at `tmuxatlas.example.com`.
`https://...` enables `Secure` authentication cookies and the browser WebAuthn
API. Do not bind the HTTP origin to a public interface. If the gateway is on
another machine or container, bind only to a protected private interface and
restrict it with firewall rules.

## First Passkey enrollment

On a new installation the server log prints a random, one-time setup token.
Open the final HTTPS URL, paste that token into the setup screen, and choose
**Create passkey**. The extra token prevents an arbitrary first public visitor
from enrolling as administrator.

Authenticator selection is handled by the browser. Depending on the browser,
operating system, and installed extensions, it can offer:

- the current device's platform Passkey;
- **Use another device**, which displays a QR code that an iPhone can scan;
- Proton Pass, Bitwarden, 1Password, or another WebAuthn-compatible provider.

TmuxAtlas does not render the QR code itself and does not contain
provider-specific integrations. Do not add Cloudflare Access solely for this
flow; ordinary Cloudflare Tunnel HTTPS reverse proxying is sufficient.

The one-time token is consumed when registration starts. If the browser cancels
or verification fails, restart TmuxAtlas to emit a fresh token. The credential
private key stays on the authenticator; TmuxAtlas stores the public credential
record in `~/.config/tmuxatlas/passkeys.json`.

## Add and manage backup Passkeys

While signed in, open **Settings → Security → Passkeys**. Enter an optional
label and choose **Add passkey**. TmuxAtlas uses the same administrator identity
and public hostname as the first credential, then lets the browser offer any
compatible provider:

- a platform Passkey on the current device;
- an iPhone through the browser's **Use another device** QR flow;
- Proton Pass, Bitwarden, 1Password, a hardware key, or another
  WebAuthn-compatible provider.

The list shows only safe metadata: label, creation time, and last-used time.
Rename credentials so their location is clear. Deleting one requires explicit
confirmation, and both the interface and server prevent deleting the final
credential. Add a backup on a separate device or provider and test a fresh
login before removing an old Passkey.

TmuxAtlas has no self-service recovery flow after all authenticators are lost.
Keep at least two independently accessible Passkeys. If none remain, an
operator with shell access must stop TmuxAtlas, move
`~/.config/tmuxatlas/passkeys.json` aside, restart the service, and use the new
one-time setup token to enroll again. This resets all prior credentials.
Restoring `passkeys.json` does not restore missing private keys.

Browser sessions use a sliding idle timeout. The default is 24 hours; each
authenticated HTTP request refreshes both the server-side session and browser
cookie. Configure it in `~/.config/tmuxatlas/.env`, for example:

```dotenv
TMUXATLAS_SESSION_TTL=168h
```

Sessions remain in memory, so restarting TmuxAtlas always requires a fresh
Passkey login regardless of this setting.

## Cloudflare Tunnel

Create a DNS route for the tunnel, then use an ingress rule like:

```yaml
tunnel: YOUR_TUNNEL_ID
credentials-file: /etc/cloudflared/YOUR_TUNNEL_ID.json

ingress:
  - hostname: tmuxatlas.example.com
    service: http://127.0.0.1:7654
    originRequest:
      httpHostHeader: tmuxatlas.example.com
      connectTimeout: 30s
      tcpKeepAlive: 30s
  - service: http_status:404
```

Run `cloudflared tunnel run YOUR_TUNNEL_ID` on the same host as TmuxAtlas. Cloudflare Tunnel supports WebSocket forwarding; the HTTP service URL is intentional because TLS terminates at Cloudflare. `httpHostHeader` preserves the public host used by same-origin WebSocket checks. No CF Access policy or service token is required by TmuxAtlas.

Verify the browser dashboard and both WebSocket routes through the public hostname. If an intermediate proxy imposes idle limits, raise them for long-lived control and terminal connections.

## Nginx with ACME

Obtain a certificate for `tmuxatlas.example.com` with your ACME client, then configure Nginx:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    server_name tmuxatlas.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tmuxatlas.example.com;

    ssl_certificate     /etc/letsencrypt/live/tmuxatlas.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tmuxatlas.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:7654;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 24h;
        proxy_send_timeout 24h;
    }
}
```

`proxy_pass` without a replacement URI preserves the complete path and query string, including the PTY `stream` parameter. The upgrade headers apply to browser, peer-control, and peer-PTY WebSockets. Reload Nginx after validating the configuration.

## Pair and run peers

Generate a short-lived code on the hub:

```bash
tmuxatlas pair
```

Join from each peer using the public, trusted gateway URL:

```bash
tmuxatlas pair --hub https://tmuxatlas.example.com --code WORD-WORD-WORD-WORD-WORD-WORD
tmuxatlas agent
tmuxatlas install --mode agent
```

Pairing saves `TMUXATLAS_HUB` in `~/.config/tmuxatlas/.env`. The Agent runs
without a TCP listener, Web UI, Passkey configuration, or public URL. It keeps
only an outbound WSS connection and a user-private Unix socket for local hooks.
The peer uses normal hostname verification and the operating-system trust store.
There is no private CA import, certificate pin, or insecure-verification switch.
Certificate rotation by Cloudflare or ACME does not require re-pairing because
Ed25519 peer identity is separate from TLS.

For controlled local development only, an explicit plaintext hub is supported:

```bash
tmuxatlas agent --hub http://127.0.0.1:7654
```

Bare hostnames default to a secure connection. A self-signed or hostname-invalid gateway certificate is rejected.

## Runtime protocol v1 and upgrade order

Multi-host control now uses a mandatory runtime-v1 hello after Ed25519
authentication. The hello negotiates a common protocol version and capabilities
before a Peer becomes online. Session mutations are correlated requests: the Hub
reports success only after the Agent returns a terminal result. Remote PTY data
uses generation-bound binary/control frames and a one-time attachment token.

This is a breaking protocol change. Upgrade one deployment as a set:

1. update and restart the Hub;
2. update and restart every Agent;
3. confirm `/api/hosts` reports `runtime_protocol: 1`, a non-zero `generation`,
   the expected `capabilities`, and the Agent build version;
4. test one remote rename and terminal, including resize, before completing the
   rollout.

An old Agent that omits the hello remains offline and the Hub logs
`protocol-incompatible`; it is never guessed to be compatible. A compatible
Agent missing an optional feature receives `capability-unsupported` for that
operation. The web UI and embedded frontend must come from the same Hub binary:
every mutation and terminal open now requires both `host_id` and `session`.
There is no missing-host fallback to Hub-local tmux, including when two machines
have sessions with the same name.

Do not roll back only one side. To roll back, stop the Hub and Agents, restore
the previous Hub binary and its matching embedded frontend, restore the same
previous Agent binary everywhere, and restore the Peer-store backup only if the
older release requires it. Interactive terminals and in-flight actions are not
migrated across an upgrade or rollback.

Agent action outcome retention defaults to five minutes, 1024 requests, and
64 KiB per terminal result/error. Operators with unusually constrained or busy
Agents may set:

```dotenv
TMUXATLAS_PEER_OUTCOME_TTL=5m
TMUXATLAS_PEER_OUTCOME_MAX_ENTRIES=1024
TMUXATLAS_PEER_OUTCOME_MAX_BYTES=65536
```

Changing the Agent process instance intentionally ends any result-unknown
in-flight action as `execution-unknown`; TmuxAtlas will not replay an
unprovable side effect after a crash.

## Live Peer revocation

`tmuxatlas peers remove <name>` first calls the running Hub through its private
Unix socket. The Hub atomically persists the authorization removal, then closes
the active control generation, pending actions, and remote PTYs. If the Hub is
not running, the command updates `peers.json` directly for the next start. A
running Hub rejection is never bypassed with a direct file write.

## systemd examples

Hub:

```ini
[Unit]
Description=TmuxAtlas web dashboard (hub)
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/tmuxatlas server
Environment=TMUXATLAS_LISTEN=127.0.0.1:7654
Environment=TMUXATLAS_PUBLIC_URL=https://tmuxatlas.example.com
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Agent:

```ini
[Unit]
Description=TmuxAtlas headless agent
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/tmuxatlas agent
Environment=TMUXATLAS_HUB=https://tmuxatlas.example.com
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Enable user lingering on headless hosts if the service must survive logout.

## Upgrade from built-in TLS

This release removes built-in certificate generation and TLS serving. The following options are rejected and must be removed from scripts and service definitions:

```text
--port
--no-tls
--tls-cert
--tls-key
--tls-san
--tls-reload-interval
--insecure
TMUXATLAS_PORT
TMUXATLAS_NO_TLS
TMUXATLAS_TLS_CERT
TMUXATLAS_TLS_KEY
TMUXATLAS_TLS_SAN
TMUXATLAS_TLS_RELOAD_INTERVAL
TMUXATLAS_INSECURE
```

Replace `--port 7654` with `--listen 127.0.0.1:7654`, and set `--public-url https://tmuxatlas.example.com`.

When TmuxAtlas first reads a legacy peer store, it:

1. creates `peers.json.pre-system-trust.bak` if a backup does not already exist;
2. preserves peer names, Ed25519 public keys, and pairing timestamps;
3. removes obsolete `ca_cert_pem` and `tls_cert_pem` fields.

Migration is idempotent. The old certificate and key files are not read or deleted. Keep the backup during rollout; after verifying browser login, pairing, peer state, and a remote terminal, unused TLS files may be deleted manually. To roll back, stop TmuxAtlas, restore the old binary and backup peer store, or re-pair peers.

After upgrading, rerun `tmuxatlas agent-setup` on every Hub and Agent. Old
`--server`, `TMUXATLAS_URL`, and `GUPPI_URL` notify settings are intentionally
unsupported because hook events are Unix-socket-only. Existing Passkey and Peer
identity files are validated but never rewritten by this migration. Pending
three-word pairing codes are invalid; generate a new six-word code. For rollback,
stop the service, restore the previous binary and its peer-store backup, then
restore the previous hook configuration only if the old binary is also restored.

## Verification and troubleshooting

Run these checks through the public hostname:

```bash
curl -I https://tmuxatlas.example.com/
tmuxatlas peers list
```

Then verify:

- Passkey enrollment/login succeeds and the session cookie is `Secure`;
- a peer remains online and its sessions update;
- opening a remote session creates a working interactive terminal;
- the gateway logs successful `101 Switching Protocols` responses for `/ws/peer` and `/ws/peer-pty`.
- `/api/hosts` shows the expected build, runtime protocol, generation, and
  capabilities for every online Agent.

If the dashboard loads but WebSockets fail, check `Host`, `Upgrade`, and `Connection` forwarding and the proxy timeouts. If peers report certificate errors, verify DNS, certificate hostname coverage, the full ACME chain, system time, and the local operating-system trust store. Do not bypass verification.
