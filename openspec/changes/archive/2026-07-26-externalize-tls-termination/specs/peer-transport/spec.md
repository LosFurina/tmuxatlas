## ADDED Requirements

### Requirement: System-trusted peer transport
TmuxAtlas peer clients SHALL connect to HTTPS/WSS hubs using normal hostname verification and the operating system trust store.

#### Scenario: Cloudflare or ACME certificate
- **WHEN** a peer connects to a hub hostname whose certificate chains to a system-trusted root
- **THEN** pairing, the peer control channel, and PTY relay connections succeed through the gateway

#### Scenario: Untrusted certificate
- **WHEN** a peer connects to a hub presenting an untrusted or hostname-invalid certificate
- **THEN** the connection fails without an insecure verification bypass

#### Scenario: Explicit local HTTP hub
- **WHEN** an operator explicitly supplies an `http://` or `ws://` hub URL
- **THEN** TmuxAtlas uses plaintext HTTP/WS for that explicitly configured local transport

### Requirement: Transport-independent peer identity
TmuxAtlas SHALL authenticate paired nodes with the existing Ed25519 identity and challenge/signature protocol independently of the gateway certificate.

#### Scenario: Valid paired peer
- **WHEN** a peer completes the challenge using the private key corresponding to its stored public key
- **THEN** the hub accepts the peer after the gateway transport is established

#### Scenario: Invalid peer signature
- **WHEN** a connection cannot sign the challenge with a paired identity
- **THEN** the hub rejects the peer even though the gateway TLS connection succeeded

### Requirement: Certificate-free pairing
TmuxAtlas SHALL complete pairing using the one-time code and Ed25519 public keys without embedding, returning, pinning, or persisting TLS certificate material.

#### Scenario: Generate pairing code
- **WHEN** a user requests a new pairing code
- **THEN** TmuxAtlas returns the expiring word code without a TLS certificate fingerprint suffix

#### Scenario: Complete pairing
- **WHEN** a peer submits a valid unexpired code and public key through a system-trusted HTTPS gateway
- **THEN** both nodes persist the peer identity without CA or leaf-certificate fields

### Requirement: Legacy peer trust migration
TmuxAtlas SHALL remove obsolete private-CA and pinned-certificate material from legacy peer records while preserving peer identity.

#### Scenario: Load a legacy peer record
- **WHEN** a stored peer contains `ca_cert_pem` or `tls_cert_pem`
- **THEN** TmuxAtlas preserves the peer name, public key, and pairing metadata while removing the legacy transport fields

#### Scenario: Repeat migration
- **WHEN** an already migrated peer store is loaded again
- **THEN** no peer identity changes and no additional migration is required

#### Scenario: Connect after migration
- **WHEN** a migrated peer connects through a gateway with a system-trusted certificate
- **THEN** the connection uses system trust and the existing Ed25519 pairing without requiring a new pairing

### Requirement: Gateway-transparent peer routes
The hub SHALL keep peer pairing, control, and PTY relay routes usable through a generic WebSocket-capable reverse proxy.

#### Scenario: Peer control channel through gateway
- **WHEN** a gateway forwards `/ws/peer` with WebSocket upgrade semantics
- **THEN** the peer maintains its long-lived state synchronization channel

#### Scenario: Peer PTY stream through gateway
- **WHEN** a gateway forwards `/ws/peer-pty` including the stream query parameter
- **THEN** the hub relays the requested PTY stream
