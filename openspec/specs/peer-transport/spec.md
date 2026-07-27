# Peer Transport Specification

## Purpose

Define trusted-gateway peer transport, Ed25519 peer identity, certificate-free
pairing, legacy trust migration, and reverse-proxy route behavior.

## Requirements

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
TmuxAtlas SHALL complete pairing using a finite-lifetime one-time code and Ed25519 identity without embedding, returning, pinning, or persisting TLS certificate material. The pairing submitter MUST prove possession of the private key by signing a versioned, domain-separated and unambiguously encoded transcript that binds the configured Hub origin, normalized code, peer name and submitted public key. Code validation, proof verification, conflict checking, one-time consumption and Peer persistence MUST have atomic success semantics.

#### Scenario: Generate pairing code
- **WHEN** a user requests a new pairing code
- **THEN** TmuxAtlas returns the expiring high-entropy word code without a TLS certificate fingerprint suffix

#### Scenario: Complete pairing
- **WHEN** a peer submits a valid unexpired code, valid identity fields and a valid transcript signature through a system-trusted HTTPS gateway
- **THEN** both nodes persist the peer identity without CA or leaf-certificate fields and the Hub consumes the code

#### Scenario: Missing private-key proof
- **WHEN** a client knows a valid code and public key but cannot produce the matching transcript signature
- **THEN** TmuxAtlas rejects the pairing without persisting the key or consuming the code

#### Scenario: Concurrent code consumption
- **WHEN** multiple validly signed requests concurrently complete the same pairing code
- **THEN** at most one request persists a Peer and succeeds

#### Scenario: Replay completed pairing
- **WHEN** a previously successful pairing completion is replayed
- **THEN** TmuxAtlas rejects it without modifying the existing Peer store

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

### Requirement: Outbound-only headless Agent
TmuxAtlas SHALL provide a dedicated Agent runtime that observes the current
user's tmux sessions and connects to the configured Hub without opening a TCP
listener or initializing the Web UI, Passkeys, WebPush, or Hub-side handlers.
The Agent SHALL retain a user-private Unix socket for local notification hooks.

#### Scenario: Start a paired Agent
- **WHEN** an operator starts `tmuxatlas agent` with a trusted Hub URL
- **THEN** the Agent synchronizes state over outbound WSS and exposes no TCP listening socket

#### Scenario: Local hook notification
- **WHEN** an AI tool hook posts an event through the Agent's Unix socket
- **THEN** the Agent records and forwards the event to the authenticated Hub connection

### Requirement: Agent user service
TmuxAtlas SHALL install the headless Agent as a same-user systemd or launchd
service with automatic restart and the paired Hub URL.

#### Scenario: Linux Agent installation
- **WHEN** an operator installs Agent mode on Linux
- **THEN** TmuxAtlas enables `tmuxatlas-agent.service` for the current user

#### Scenario: macOS Agent installation
- **WHEN** an operator installs Agent mode on macOS
- **THEN** TmuxAtlas loads `com.tmuxatlas.agent` for the current user

#### Scenario: Agent update
- **WHEN** the updater replaces the executable used by a running Agent service
- **THEN** it restarts that Agent service without reporting browser-session invalidation

### Requirement: Pairing abuse resistance
TmuxAtlas SHALL generate pairing codes with at least 48 bits of minimum entropy using unbiased cryptographic random sampling, SHALL expire them after a finite short lifetime, and SHALL bound pending codes, per-source failures, global attempts and concurrent completions. Exhausting a limit MUST NOT consume a valid code.

#### Scenario: Generate unbiased pairing code
- **WHEN** an operator requests a pairing code while pending capacity is available
- **THEN** TmuxAtlas returns a code produced without modulo bias and having at least 48 bits of minimum entropy

#### Scenario: Pairing brute-force attempt
- **WHEN** a source repeatedly submits unsuccessful pairing completions
- **THEN** TmuxAtlas rate-limits that source and the global pairing budget before the code space can be enumerated

#### Scenario: Pending code capacity reached
- **WHEN** unexpired pending codes have reached the finite configured limit
- **THEN** TmuxAtlas rejects generation without evicting an unexpired code

### Requirement: Strict Ed25519 identity validation
TmuxAtlas SHALL validate canonical Base64 encoding, exact Ed25519 public-key, private-key and signature lengths, local public/private consistency, bounded peer names, duplicate public keys and conflicting peer names before persistence or any Ed25519 operation. Malformed persisted identity data MUST produce an actionable startup or doctor error and MUST NOT reach a signing or verification call.

#### Scenario: Malformed pairing key
- **WHEN** pairing receives non-canonical Base64 or a decoded public key whose length is not the Ed25519 public-key size
- **THEN** TmuxAtlas rejects the pairing generically without changing the Peer store or panicking

#### Scenario: Malformed persisted local identity
- **WHEN** the local identity contains a wrong-length private key or a public key that does not match it
- **THEN** startup fails with an actionable local diagnostic before opening public services

#### Scenario: Conflicting peer identity
- **WHEN** a new pairing reuses an existing public key or conflicts with an existing peer name
- **THEN** TmuxAtlas deterministically rejects it and preserves the existing trust record

#### Scenario: Malformed persisted peer
- **WHEN** the Peer store contains an invalid key, duplicate key or conflicting name
- **THEN** startup and doctor identify the affected local record without automatically rewriting trust data
