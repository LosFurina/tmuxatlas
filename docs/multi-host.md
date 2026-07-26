# Multi-Host and Trusted Gateway Deployment

TmuxAtlas can aggregate tmux sessions from several machines into one dashboard. The hub and peers use an application-level Ed25519 identity; public TLS belongs to a trusted gateway such as Cloudflare Tunnel or Nginx with an ACME certificate.

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
tmuxatlas pair generate
```

Join from each peer using the public, trusted gateway URL:

```bash
tmuxatlas pair join --hub https://tmuxatlas.example.com --code WORD-WORD-WORD
tmuxatlas server --hub https://tmuxatlas.example.com
```

The peer uses normal hostname verification and the operating-system trust store. There is no private CA import, certificate pin, or insecure-verification switch. Certificate rotation by Cloudflare or ACME does not require re-pairing because Ed25519 peer identity is separate from TLS.

For controlled local development only, an explicit plaintext hub is supported:

```bash
tmuxatlas server --hub http://127.0.0.1:7654
```

Bare hostnames default to a secure connection. A self-signed or hostname-invalid gateway certificate is rejected.

Use `--local-only` on a peer if its local dashboard should show only that machine while the hub still receives its sessions:

```bash
tmuxatlas server --hub https://tmuxatlas.example.com --local-only
```

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

Peer:

```ini
[Unit]
Description=TmuxAtlas web dashboard (peer)
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/tmuxatlas server
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

If the dashboard loads but WebSockets fail, check `Host`, `Upgrade`, and `Connection` forwarding and the proxy timeouts. If peers report certificate errors, verify DNS, certificate hostname coverage, the full ACME chain, system time, and the local operating-system trust store. Do not bypass verification.
