## ADDED Requirements

### Requirement: Expiring first-registration bootstrap
TmuxAtlas SHALL create a cryptographically random bootstrap token only while no Passkey exists, retain only its digest in memory, expire it after a finite short lifetime, and disclose its plaintext only once through controlled startup output or a same-user Unix-socket management operation. The token MUST bind atomically to one registration ceremony and MUST become unusable when that ceremony succeeds, fails, is cancelled or expires.

#### Scenario: Initial bootstrap token
- **WHEN** the Hub starts with an empty Passkey store
- **THEN** it creates a time-limited token, stores only its digest and emits the plaintext at most once outside routine request logs

#### Scenario: Bind bootstrap ceremony
- **WHEN** a client presents a valid unbound bootstrap token to begin first registration
- **THEN** TmuxAtlas binds it to exactly one new registration ceremony and rejects another begin using the same token

#### Scenario: Bootstrap ceremony ends
- **WHEN** the bound registration ceremony succeeds, fails, is cancelled or expires
- **THEN** its bootstrap token cannot begin or finish another ceremony

#### Scenario: First Passkey persisted
- **WHEN** the first credential is atomically persisted
- **THEN** TmuxAtlas immediately invalidates every bootstrap token and unfinished bootstrap ceremony

### Requirement: Local bootstrap rotation
TmuxAtlas SHALL allow bootstrap rotation only through a same-user private Unix-socket management operation and only while the Passkey store is empty. Rotation MUST invalidate previous bootstrap tokens and ceremonies and MUST NOT require deleting the credential store or restarting the Hub.

#### Scenario: Rotate expired token locally
- **WHEN** a same-user operator requests rotation after a bootstrap token expires and no Passkey exists
- **THEN** TmuxAtlas invalidates prior bootstrap state and returns one new expiring token through the local channel

#### Scenario: Remote rotation attempt
- **WHEN** a client requests bootstrap rotation through the public TCP listener
- **THEN** no rotation route is available and bootstrap state remains unchanged

#### Scenario: Rotation after enrollment
- **WHEN** an operator requests bootstrap rotation while at least one Passkey exists
- **THEN** TmuxAtlas rejects the request without generating a token

### Requirement: Bounded WebAuthn ceremony state
TmuxAtlas SHALL rate-limit and cap public WebAuthn begin/finish operations and total unexpired ceremony state. Ceremony cleanup and lookup MUST have bounded work per request, and a rejected request MUST NOT create ceremony state.

#### Scenario: Ceremony capacity reached
- **WHEN** unexpired registration or login ceremonies reach the configured finite capacity
- **THEN** TmuxAtlas rejects another begin without allocating a ceremony

#### Scenario: Authentication begin flood
- **WHEN** a source exceeds the allowed WebAuthn begin rate or concurrency
- **THEN** TmuxAtlas rejects the excess requests before generating WebAuthn options or ceremony records

#### Scenario: Ceremony expiry cleanup
- **WHEN** ceremonies expire
- **THEN** TmuxAtlas removes them with bounded incremental work and makes capacity available again
