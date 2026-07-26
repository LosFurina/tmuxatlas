## Why

TmuxAtlas currently generates and distributes a private certificate authority by default, forcing users to bypass browser warnings or install a project-controlled root certificate. Deployments already have a trusted TLS boundary such as Cloudflare Tunnel or Nginx with ACME, so TmuxAtlas should focus on application authentication and peer identity while delegating public TLS termination to that boundary.

## What Changes

- **BREAKING** Remove TmuxAtlas's HTTPS listener, self-signed CA and server certificate generation, certificate hot reload, CA download endpoints, certificate trust UI, and all server-side TLS flags and environment variables.
- **BREAKING** Bind the HTTP server to loopback by default and replace the port-only setting with an explicit listen address that operators can override.
- Add an external public URL setting so TmuxAtlas can mark session cookies `Secure` and report the correct externally reachable URL while receiving plain HTTP from a trusted local proxy.
- Preserve browser HTTPS/WSS behavior through Cloudflare Tunnel and conventional reverse proxies, with documented Host preservation and WebSocket upgrade requirements.
- Keep peer connections on standard HTTPS/WSS using the operating system trust store, while removing private-CA trust, leaf-certificate pinning, pairing-code certificate fingerprints, and insecure verification bypasses.
- Migrate existing peer records by discarding legacy CA and pinned-certificate material without discarding Ed25519 identities or requiring a new pairing when the peer identity remains valid.
- Update installation output, examples, multi-host guidance, and security documentation for proxy-terminated TLS.

## Capabilities

### New Capabilities

- `proxy-deployment`: Defines loopback-safe HTTP serving behind Cloudflare Tunnel or a conventional trusted reverse proxy, including public URL, secure cookies, Host handling, and WebSocket forwarding expectations.
- `peer-transport`: Defines peer pairing and long-lived peer connectivity over system-trusted HTTPS/WSS without TmuxAtlas-managed certificate authorities or certificate pinning.

### Modified Capabilities

None. The project does not yet contain baseline OpenSpec capability specifications.

## Impact

- Backend server startup, listener configuration, authentication cookies, pairing responses, and peer client transports.
- Removal of `pkg/tlscert`, its tests, server TLS options, certificate endpoints, and certificate reload infrastructure.
- Peer store schema compatibility and migration behavior for existing `ca_cert_pem` and `tls_cert_pem` values.
- Frontend routing and removal of the certificate trust component.
- CLI flags, environment variables, service installation output, README, and multi-host documentation.
- Deployments that directly use TmuxAtlas's built-in HTTPS or expose port 7654 remotely must move TLS to a trusted gateway and update their configuration.
