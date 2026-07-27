## ADDED Requirements

### Requirement: Fail-closed unauthenticated mode
TmuxAtlas SHALL allow `--no-auth` only when every configured listener address is loopback and the Public URL resolves syntactically to an explicitly allowed localhost or loopback origin. Startup MUST reject unauthenticated mode combined with an external origin, wildcard or non-loopback listener, or a reverse-proxy deployment configuration.

#### Scenario: Loopback development mode
- **WHEN** an operator enables `--no-auth` with a loopback listener and an allowed localhost or loopback Public URL
- **THEN** TmuxAtlas starts the explicitly local unauthenticated development service

#### Scenario: External unauthenticated origin
- **WHEN** `--no-auth` is combined with an external HTTPS Public URL
- **THEN** startup fails with an actionable configuration error

#### Scenario: Non-loopback unauthenticated listener
- **WHEN** `--no-auth` is combined with a wildcard or non-loopback listen address
- **THEN** startup fails rather than exposing unauthenticated terminal routes

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
TmuxAtlas SHALL enforce authentication, Host and Origin validation, request and connection limits, and protocol-specific cryptographic checks inside the application even when a trusted gateway is configured. Deployment documentation MUST describe gateway controls as additional defense rather than substitutes for application checks.

#### Scenario: Permissive reverse proxy
- **WHEN** a gateway forwards a request that violates an application ingress rule
- **THEN** TmuxAtlas rejects it even though the gateway accepted it

#### Scenario: Deployment guidance
- **WHEN** an operator follows a supported Cloudflare Tunnel or Nginx deployment
- **THEN** the documentation requires application authentication and preserved Host/Origin behavior and does not recommend public `--no-auth`

## MODIFIED Requirements

### Requirement: Proxy-compatible browser WebSockets
TmuxAtlas SHALL support browser WebSocket connections forwarded by a trusted HTTP reverse proxy while requiring both the forwarded request Host and browser Origin to match the normalized `TMUXATLAS_PUBLIC_URL` origin and retaining normal authentication.

#### Scenario: Preserved public Host
- **WHEN** a gateway forwards a WebSocket request whose Host and browser Origin both exactly match the configured Public URL origin
- **THEN** TmuxAtlas accepts the upgrade subject to normal authentication and WebSocket ingress limits

#### Scenario: Mismatched public Host
- **WHEN** a browser WebSocket Origin or forwarded request Host does not match the configured Public URL origin
- **THEN** TmuxAtlas rejects the upgrade
