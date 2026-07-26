## Context

TmuxAtlas currently combines three responsibilities on one listener: browser delivery, peer WebSocket transport, and TLS certificate lifecycle management. When no certificate is supplied, it creates a long-lived private CA, signs a server certificate, exposes endpoints that distribute the CA, and guides users to install that CA into operating-system trust stores. Peer pairing also carries CA material or pins a leaf certificate.

The intended deployments terminate public TLS at either Cloudflare Tunnel or a conventional reverse proxy such as Nginx with an ACME certificate. Both gateways can accept HTTPS/WSS and forward HTTP/WS to a loopback origin. TmuxAtlas must therefore remain secure behind a trusted local gateway, continue to authenticate browser sessions and Ed25519 peer identities, and avoid coupling its protocol to a particular gateway vendor.

## Goals / Non-Goals

**Goals:**

- Make TmuxAtlas an HTTP/WS origin with no certificate generation, storage, distribution, reload, or server-side TLS configuration.
- Default to a loopback-only listener so removal of built-in TLS cannot accidentally expose plaintext terminal access on the network.
- Preserve secure browser sessions when the public request is HTTPS but the origin request is HTTP.
- Keep peer pairing, state synchronization, and PTY relay working through trusted HTTPS/WSS gateways.
- Replace private-CA and pinned-certificate verification with the Go/system trust store.
- Migrate legacy peer records without losing Ed25519 peer identities.
- Support Cloudflare Tunnel and ordinary reverse proxies through generic HTTP and WebSocket behavior rather than vendor-specific code.

**Non-Goals:**

- Managing Cloudflare Access, service tokens, ACME issuance, Nginx, DNS, or gateway lifecycle.
- Accepting Proxy Protocol or trusting forwarded headers for authorization.
- Replacing TmuxAtlas password authentication or Ed25519 peer authentication with gateway authentication.
- Providing end-to-end TLS between a gateway and a loopback TmuxAtlas origin.
- Supporting TLS termination inside the TmuxAtlas process after this breaking change.

## Decisions

### 1. Serve HTTP/WS only on an explicit listen address

The server will use a single `listen` string, defaulting to `127.0.0.1:7654`, and call `http.Server.Serve` directly. `TMUXATLAS_LISTEN` and `--listen` replace the port-only configuration. The old server TLS flags and environment variables are removed.

Loopback is the safe default for both Cloudflare Tunnel running on the same host and Nginx proxying locally. Operators who intentionally place the origin on a protected container or private network can override the address.

Alternative considered: retain optional built-in TLS for direct deployments. Rejected because it preserves the certificate lifecycle, UI, tests, and ambiguous security ownership that this change is intended to remove.

### 2. Use an explicit public URL as the external security signal

`--public-url` / `TMUXATLAS_PUBLIC_URL` will accept an absolute `http` or `https` URL. TmuxAtlas will use it for operator-facing URLs and to decide whether authentication cookies carry the `Secure` attribute. The default will represent local HTTP access.

TmuxAtlas will not unconditionally trust `X-Forwarded-Proto`, because a user can deliberately bind the origin beyond loopback and forwarded headers are attacker-controlled unless a trusted-proxy boundary is configured correctly. An explicit public URL is deterministic across Cloudflare Tunnel and Nginx and avoids vendor-specific logic.

Alternative considered: infer external HTTPS solely from `X-Forwarded-Proto`. Rejected because it creates a spoofable security/configuration signal and requires a trusted-proxy allowlist.

### 3. Keep same-origin WebSocket validation and require Host preservation

Browser WebSockets will continue comparing the browser `Origin` host to the request `Host`. Gateways must preserve the public Host when forwarding and must proxy WebSocket upgrades for all `/ws/*` routes. Documentation will include Cloudflare Tunnel origin and Nginx WebSocket examples, long-lived connection timeout guidance, and a recommendation to deploy at a dedicated hostname rather than under a path prefix.

Alternative considered: disable origin checking behind proxies. Rejected because it would weaken protection against cross-site WebSocket hijacking on a service that exposes interactive terminals.

### 4. Retain standard client TLS and remove custom trust

Peer clients will retain `https://` and `wss://` support. For TLS connections they will use the standard Go HTTP/WebSocket transports, which validate the gateway certificate against system roots and perform normal hostname verification.

Private CA pools, leaf-certificate pins, post-authentication pin rotation, fingerprint prefixes, and insecure verification bypasses will be removed. Bare hub addresses continue to select secure HTTPS/WSS by default; explicit `http://` or `ws://` remains available for controlled local development.

Alternative considered: keep certificate pinning as an additional layer. Rejected because gateway certificates legitimately rotate, system trust already authenticates the configured hostname, and Ed25519 authenticates the paired TmuxAtlas node at the application layer.

### 5. Keep Peer identity independent from transport certificates

The one-time pairing code will establish trust in the peer's Ed25519 public key. Subsequent control and PTY WebSockets will continue using the existing challenge/signature exchange. Pairing responses will no longer distribute CA material, and generated pairing codes will no longer append a TLS fingerprint.

This makes the separation explicit: the gateway authenticates the network endpoint with a publicly trusted certificate, while TmuxAtlas authenticates the logical peer with Ed25519.

### 6. Perform an idempotent legacy peer-store migration

On loading a peer store, TmuxAtlas will detect legacy `ca_cert_pem` and `tls_cert_pem` values, create a one-time backup before rewriting when feasible, remove only those transport-trust fields, and preserve peer names, public keys, pairing timestamps, and other identity metadata. Migration must be safe to repeat.

Legacy TLS files under the TmuxAtlas configuration directory will no longer be read. They will be left in place rather than deleted automatically, avoiding destructive cleanup and allowing an operator to recover them during rollback.

### 7. Remove certificate-specific UI and API surface

The `/api/tls/status`, `/api/tls/ca.crt`, and `/api/tls/ca.mobileconfig` endpoints, mobile configuration generation, `/trust` route, Trust Certificate component, and login-page trust links will be removed. Installation output and documentation will point local users to HTTP and remote users to their configured public URL.

## Risks / Trade-offs

- **Existing direct-HTTPS deployments stop working** → Mark the release as breaking, document gateway migration, and fail clearly on removed flags rather than silently ignoring them.
- **An operator overrides the loopback listener and exposes plaintext HTTP** → Keep loopback as the default and document that non-loopback binding requires a protected origin network.
- **Secure cookies are misconfigured because `public-url` is left at HTTP** → Document `TMUXATLAS_PUBLIC_URL` as required for gateway deployments and log a startup warning when a non-loopback listener or proxy-oriented setup lacks an HTTPS public URL.
- **A gateway rewrites Host or does not support WebSocket upgrades** → Preserve strict origin checking and provide tested Cloudflare Tunnel and Nginx configuration examples.
- **Gateway certificate rotation breaks old certificate pins** → Remove legacy pins and use system trust, for which normal ACME and Cloudflare certificate rotation is transparent.
- **Legacy peer-store rewriting complicates rollback** → Back up the store before removing legacy transport fields and leave generated TLS files untouched.
- **Cloudflare or Nginx terminates TLS before the origin** → Treat the gateway host as part of the trusted computing boundary and require loopback or a separately protected origin network.

## Migration Plan

1. Configure a Cloudflare Tunnel hostname or Nginx+ACME virtual host that forwards HTTP and WebSocket traffic to `127.0.0.1:7654`.
2. Set `TMUXATLAS_LISTEN=127.0.0.1:7654` and `TMUXATLAS_PUBLIC_URL=https://<public-host>`.
3. Upgrade TmuxAtlas. On first load, migrate legacy peer trust fields while retaining Ed25519 pairing identities.
4. Verify browser login, Secure cookies, browser terminal WebSockets, peer control WebSockets, and remote PTY relay through the public hostname.
5. Remove obsolete TLS flags and environment variables from service definitions.
6. After a stable period, operators may manually delete unused legacy TLS files.

Rollback requires restoring the previous binary and, if peer transport fields are needed again, restoring the peer-store backup or re-pairing peers. Existing TLS files remain available because the migration does not delete them.

## Open Questions

None. This breaking change removes `--port` in favor of `--listen`. Supplying a non-loopback `--listen` value is treated as the operator's explicit acknowledgement, and TmuxAtlas emits a plaintext-exposure warning rather than requiring an additional flag.
