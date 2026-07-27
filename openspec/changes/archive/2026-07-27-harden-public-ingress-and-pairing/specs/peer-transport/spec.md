## ADDED Requirements

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

## MODIFIED Requirements

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
