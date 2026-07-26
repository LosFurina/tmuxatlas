## ADDED Requirements

### Requirement: HTTP-only origin server
TmuxAtlas SHALL serve its browser, API, and WebSocket routes over HTTP/WS without generating, loading, reloading, or serving TLS certificates or certificate-authority material.

#### Scenario: Start the default origin
- **WHEN** an operator starts `tmuxatlas server` without transport-security options
- **THEN** TmuxAtlas serves HTTP and WS directly without creating certificate files

#### Scenario: Removed TLS configuration is supplied
- **WHEN** an operator supplies a removed built-in TLS flag or environment variable
- **THEN** TmuxAtlas fails with a clear unsupported-configuration error rather than silently enabling or ignoring built-in TLS

### Requirement: Loopback-safe listener
TmuxAtlas SHALL listen on `127.0.0.1:7654` by default and SHALL allow an operator to provide an explicit listen address.

#### Scenario: Default listener
- **WHEN** no listen address is configured
- **THEN** the TCP listener is reachable only through IPv4 loopback on port 7654

#### Scenario: Explicit protected-network listener
- **WHEN** an operator configures a different valid listen address
- **THEN** TmuxAtlas binds exactly to that address and reports it at startup

#### Scenario: Invalid listener
- **WHEN** the configured listen address cannot be parsed or bound
- **THEN** startup fails with an actionable error

### Requirement: External public URL
TmuxAtlas SHALL accept an absolute HTTP or HTTPS public URL describing the address exposed by the trusted gateway.

#### Scenario: HTTPS public URL
- **WHEN** `public-url` uses the `https` scheme
- **THEN** TmuxAtlas marks authentication session cookies `Secure`

#### Scenario: Local HTTP public URL
- **WHEN** `public-url` uses the `http` scheme
- **THEN** TmuxAtlas permits local HTTP login without marking the session cookie `Secure`

#### Scenario: Invalid public URL
- **WHEN** `public-url` is not absolute or uses a scheme other than HTTP or HTTPS
- **THEN** TmuxAtlas rejects the configuration during startup

### Requirement: Proxy-compatible browser WebSockets
TmuxAtlas SHALL support browser WebSocket connections forwarded by a trusted HTTP reverse proxy while retaining same-origin validation.

#### Scenario: Preserved public Host
- **WHEN** a gateway forwards a WebSocket request with the public Host matching the browser Origin
- **THEN** TmuxAtlas accepts the upgrade subject to normal authentication

#### Scenario: Mismatched public Host
- **WHEN** a browser WebSocket Origin does not match the forwarded request Host
- **THEN** TmuxAtlas rejects the upgrade

### Requirement: Gateway deployment documentation
The project SHALL document supported Cloudflare Tunnel and Nginx+ACME deployment patterns for the HTTP-only origin.

#### Scenario: Cloudflare Tunnel deployment
- **WHEN** an operator follows the Cloudflare Tunnel example
- **THEN** the tunnel forwards the public hostname to the loopback HTTP origin with a preserved Host

#### Scenario: Nginx deployment
- **WHEN** an operator follows the Nginx example
- **THEN** Nginx terminates trusted TLS and forwards HTTP plus WebSocket upgrade headers with a long-lived read timeout
