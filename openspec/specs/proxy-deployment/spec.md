# Proxy Deployment Specification

## Purpose

Define the HTTP-only origin, safe listener and public URL configuration,
reverse-proxy WebSocket behavior, and trusted-gateway deployment documentation.

## Requirements

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
TmuxAtlas SHALL support browser WebSocket connections forwarded by a trusted HTTP reverse proxy while requiring both the forwarded request Host and browser Origin to match the normalized `TMUXATLAS_PUBLIC_URL` origin and retaining normal authentication.

#### Scenario: Preserved public Host
- **WHEN** a gateway forwards a WebSocket request whose Host and browser Origin both exactly match the configured Public URL origin
- **THEN** TmuxAtlas accepts the upgrade subject to normal authentication and WebSocket ingress limits

#### Scenario: Mismatched public Host
- **WHEN** a browser WebSocket Origin or forwarded request Host does not match the configured Public URL origin
- **THEN** TmuxAtlas rejects the upgrade

### Requirement: Gateway deployment documentation
The project SHALL document supported Cloudflare Tunnel and Nginx+ACME deployment patterns for the HTTP-only origin.

#### Scenario: Cloudflare Tunnel deployment
- **WHEN** an operator follows the Cloudflare Tunnel example
- **THEN** the tunnel forwards the public hostname to the loopback HTTP origin with a preserved Host

#### Scenario: Nginx deployment
- **WHEN** an operator follows the Nginx example
- **THEN** Nginx terminates trusted TLS and forwards HTTP plus WebSocket upgrade headers with a long-lived read timeout

### Requirement: Role-aware installation
The one-line installer SHALL ask for the machine role before requesting a URL.
Hub installation SHALL request and persist a browser-facing public URL. Agent
installation SHALL request and persist only the Hub URL and pairing code.
Binary-only installation SHALL not create configuration or a user service.

#### Scenario: Interactive Hub installation
- **WHEN** an operator selects Hub
- **THEN** the installer configures the public Passkey origin and installs the Hub user service

#### Scenario: Interactive Agent installation
- **WHEN** an operator selects Agent
- **THEN** the installer pairs with the Hub and installs the outbound-only Agent service without requesting a public URL

#### Scenario: Binary-only installation
- **WHEN** an operator selects binary-only
- **THEN** the installer verifies and installs the executable without writing `.env` or starting a service

### Requirement: Operator-controlled unauthenticated mode
TmuxAtlas SHALL accept `--no-auth` with any syntactically valid listener address and Public URL. `--no-auth` disables application authentication and public Host/Origin gating for every reachable client; the operator is responsible for selecting the exposure boundary and deciding whether an external listener, private network, or trusted gateway is appropriate.

#### Scenario: Loopback unauthenticated mode
- **WHEN** an operator enables `--no-auth` with a loopback listener and Public URL
- **THEN** TmuxAtlas starts without application authentication

#### Scenario: External unauthenticated origin
- **WHEN** an operator enables `--no-auth` with an external HTTPS Public URL
- **THEN** TmuxAtlas starts without application authentication and leaves the exposure decision to the operator

#### Scenario: Non-loopback unauthenticated listener
- **WHEN** an operator enables `--no-auth` with a wildcard or non-loopback listen address
- **THEN** TmuxAtlas starts without application authentication and leaves the exposure decision to the operator

### Requirement: Configured public Host enforcement
The public TCP Router SHALL accept a request only when its HTTP `Host` matches the normalized authority of `TMUXATLAS_PUBLIC_URL`, except for explicitly enumerated localhost or loopback aliases in local development. TmuxAtlas MUST NOT use an untrusted forwarded-host header to make this authorization decision.

#### Scenario: Configured gateway Host
- **WHEN** a trusted gateway preserves the authority configured in the Public URL
- **THEN** TmuxAtlas routes the request subject to normal authentication and ingress checks

#### Scenario: DNS rebinding Host
- **WHEN** a request reaches the listener with an unconfigured Host
- **THEN** TmuxAtlas rejects it before serving API, static or WebSocket content

#### Scenario: Spoofed forwarded Host
- **WHEN** a request has an invalid HTTP Host but supplies the configured value only in `X-Forwarded-Host`
- **THEN** TmuxAtlas rejects it

### Requirement: Application-owned gateway security boundary
When `--no-auth` is not enabled, TmuxAtlas SHALL enforce authentication, Host and Origin validation, request and connection limits, and protocol-specific cryptographic checks inside the application even when a trusted gateway is configured. Deployment documentation MUST describe gateway controls as additional defense rather than substitutes for application checks.

#### Scenario: Permissive reverse proxy
- **WHEN** a gateway forwards a request that violates an application ingress rule
- **THEN** TmuxAtlas rejects it even though the gateway accepted it

#### Scenario: Deployment guidance
- **WHEN** an operator follows a supported Cloudflare Tunnel or Nginx deployment
- **THEN** the documentation describes the operator's responsibility for `--no-auth` and preserves Host/Origin behavior
