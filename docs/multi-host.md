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
- TmuxAtlas password authentication protects browser access.
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

`TMUXATLAS_PUBLIC_URL=https://...` enables `Secure` authentication cookies. Do not bind the HTTP origin to a public interface. If the gateway is on another machine or container, bind only to a protected private interface and restrict it with firewall rules.

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

- browser login succeeds and the session cookie is `Secure`;
- a peer remains online and its sessions update;
- opening a remote session creates a working interactive terminal;
- the gateway logs successful `101 Switching Protocols` responses for `/ws/peer` and `/ws/peer-pty`.

If the dashboard loads but WebSockets fail, check `Host`, `Upgrade`, and `Connection` forwarding and the proxy timeouts. If peers report certificate errors, verify DNS, certificate hostname coverage, the full ACME chain, system time, and the local operating-system trust store. Do not bypass verification.
