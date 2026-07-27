# Public Ingress Security Specification

## Purpose

Define the application-owned security boundary for public HTTP, WebSocket,
WebAuthn, pairing, bootstrap, and browser mutation traffic.

## Requirements

### Requirement: Local-only tool event ingest
TmuxAtlas SHALL expose `/api/tool-event` only on the current user's private Unix socket and MUST NOT register that route on the public TCP Router. `tmuxatlas notify` MUST use the Unix socket and MUST NOT fall back to an unauthenticated TCP endpoint.

#### Scenario: Local hook event
- **WHEN** a same-user tool hook submits a valid event through the TmuxAtlas Unix socket
- **THEN** TmuxAtlas records and forwards the event according to the running Hub or Agent role

#### Scenario: Public tool event attempt
- **WHEN** a client requests `/api/tool-event` through the TCP listener or reverse proxy
- **THEN** TmuxAtlas returns not found without recording an event or sending a Push notification

#### Scenario: Unix socket unavailable
- **WHEN** `tmuxatlas notify` cannot connect to the configured Unix socket
- **THEN** the command fails clearly without attempting an HTTP or HTTPS fallback

### Requirement: Bounded public HTTP requests
The public TCP server SHALL apply finite header size, header/body read duration, global request-body size and stricter route-specific body limits before decoding request data. JSON handlers MUST reject trailing JSON values and MUST NOT allocate or retain data beyond the applicable bound.

#### Scenario: Oversized route body
- **WHEN** a request body exceeds the finite limit assigned to its public route
- **THEN** TmuxAtlas returns payload too large and does not invoke the route's state mutation

#### Scenario: Slow or incomplete request
- **WHEN** a client does not complete public request headers or body within the configured finite deadline
- **THEN** TmuxAtlas terminates the request and releases its request resources

#### Scenario: Multiple JSON values
- **WHEN** a JSON endpoint receives one valid value followed by additional JSON data
- **THEN** TmuxAtlas rejects the request without applying a partial mutation

### Requirement: Public ingress abuse controls
TmuxAtlas SHALL enforce finite per-source and global rate, burst and in-flight concurrency limits for public WebAuthn begin/finish, bootstrap, pairing and WebSocket upgrade operations. Source identity MUST be derived from the direct network peer unless an explicitly trusted proxy policy is configured, and forwarded client-address headers MUST NOT be trusted by default.

#### Scenario: Per-source burst exhausted
- **WHEN** one direct source exceeds an endpoint category's allowed burst or sustained rate
- **THEN** TmuxAtlas rejects subsequent attempts with a stable rate-limit response until capacity recovers

#### Scenario: Global concurrency exhausted
- **WHEN** accepted in-flight operations reach an endpoint category's global concurrency limit
- **THEN** TmuxAtlas rejects additional work without creating ceremony, pairing or connection state

#### Scenario: Spoofed forwarded address
- **WHEN** an untrusted direct client changes `X-Forwarded-For` or a similar forwarded-address header
- **THEN** the request remains charged to the direct network source

### Requirement: Bounded WebSocket lifecycle
TmuxAtlas SHALL apply finite handshake deadlines, connection limits, inbound message limits and liveness deadlines independently to browser, Peer control and Peer PTY WebSockets. A rejected or abnormal connection MUST release every acquired connection slot.

#### Scenario: Oversized WebSocket message
- **WHEN** an authenticated WebSocket sends a message larger than its protocol-specific read limit
- **THEN** TmuxAtlas closes that connection without processing the oversized message

#### Scenario: Stale WebSocket
- **WHEN** a WebSocket does not satisfy its protocol's ping/pong or activity deadline
- **THEN** TmuxAtlas closes it and releases its connection capacity

#### Scenario: Connection limit reached
- **WHEN** a WebSocket category has reached its finite connection limit
- **THEN** a new upgrade is rejected before long-lived connection state is allocated

### Requirement: Same-origin browser mutation protection
Every cookie-authenticated browser mutation SHALL require an `Origin` whose normalized scheme, host and effective port exactly match `TMUXATLAS_PUBLIC_URL`, and a CSRF token bound to the active Session. A route with a JSON request body MUST additionally require an `application/json` media type. Routes without a browser Session MUST use a narrowly enumerated alternative protection and MUST NOT receive a blanket CSRF bypass.

#### Scenario: Valid same-origin mutation
- **WHEN** an authenticated browser sends a mutation with the configured Origin, its Session's CSRF token and the required JSON Content-Type
- **THEN** TmuxAtlas evaluates and applies the requested mutation

#### Scenario: Same-site different-origin mutation
- **WHEN** an authenticated browser request originates from a different origin under the same registrable site
- **THEN** TmuxAtlas rejects it even if the browser includes the Session cookie

#### Scenario: CORS-safelisted content type
- **WHEN** a JSON mutation is sent as `text/plain`, form data or another non-JSON media type
- **THEN** TmuxAtlas rejects it before decoding or changing state

#### Scenario: Explicit non-browser protocol
- **WHEN** the Unix socket, Peer protocol or pre-Session WebAuthn ceremony uses its documented independent trust mechanism
- **THEN** only the specifically registered route bypasses the Session CSRF token while retaining its own Origin, cryptographic and resource checks

### Requirement: Non-disclosing ingress failures
TmuxAtlas SHALL return stable error classes for body, media-type, rate, concurrency, Host, Origin and CSRF failures, and SHALL emit aggregate security metrics without logging request secrets. Pairing and bootstrap failures MUST NOT reveal whether a submitted secret exists, expired or failed a later validation step.

#### Scenario: Invalid pairing secret
- **WHEN** a pairing request uses an unknown or expired code, an invalid proof, or a conflicting identity
- **THEN** the remote response has the same generic failure shape and contains no validation-stage detail

#### Scenario: Security event recording
- **WHEN** ingress rejects a request for a security limit or trust-boundary violation
- **THEN** TmuxAtlas increments a reason-and-route aggregate and omits bodies, Passkey data, bootstrap tokens, pairing codes, signatures and Push keys
