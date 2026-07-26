## ADDED Requirements

### Requirement: Official pure Hub OCI image
The project SHALL provide an official multi-stage OCI image whose default process is the pure `tmuxatlas hub` runtime. The runtime image MUST contain the embedded application, system CA roots and a non-root user, and MUST NOT install or require tmux, a shell, a compiler or a package manager.

#### Scenario: Start the image without tmux
- **WHEN** the official image starts with its required environment and persistent volume
- **THEN** the Hub becomes ready without a tmux executable or host tmux socket

#### Scenario: Inspect runtime privilege
- **WHEN** CI inspects the running official image
- **THEN** the main process uses the documented non-root UID and does not require privileged mode, host PID/network, Docker socket or added Linux capabilities

#### Scenario: Use system trust for outbound HTTPS
- **WHEN** Hub sends Web Push or performs an allowed HTTPS request
- **THEN** the runtime uses bundled system CA roots and does not rely on a generated or self-signed application certificate

### Requirement: Secure single-service Compose deployment
The project SHALL provide a single-Hub `compose.yaml` and Docker environment example. The service MUST listen on `0.0.0.0:7654` inside the container, publish by default only to host `127.0.0.1:7654`, require the final absolute `TMUXATLAS_PUBLIC_URL`, mount one persistent data volume, use a restart policy and bounded stop grace period, and define a role-aware healthcheck.

#### Scenario: Validate Compose configuration
- **WHEN** operator supplies the required Public URL and runs `docker compose config`
- **THEN** one Hub service, one persistent volume, loopback port publication, healthcheck, restart policy and grace period resolve without an external database/cache service

#### Scenario: Public URL is missing
- **WHEN** operator attempts the documented Compose deployment without `TMUXATLAS_PUBLIC_URL`
- **THEN** configuration or startup fails with an actionable message rather than deriving a WebAuthn origin from the internal container address

#### Scenario: Run with hardened defaults
- **WHEN** the documented Compose service starts
- **THEN** it supports a read-only root filesystem, temporary `/tmp` and `/run/tmuxatlas`, all capability drop and `no-new-privileges`

#### Scenario: Attempt multiple replicas
- **WHEN** operator configures more than one active Hub replica against the same file volume
- **THEN** documentation and diagnostics report the unsupported topology and do not claim multi-writer safety

### Requirement: Trusted gateway boundary
Docker deployment SHALL terminate TLS at a system-trusted external gateway such as Nginx with ACME or Cloudflare Tunnel. The container MUST serve origin HTTP/WSS only, MUST NOT generate self-signed certificates, and SHALL retain the Host/Origin and authentication requirements defined by `proxy-deployment` and public ingress security.

#### Scenario: Proxy through a trusted gateway
- **WHEN** browser and Agent traffic reaches the loopback-bound Hub through the configured HTTPS gateway with the final authority preserved
- **THEN** Passkey, browser WebSocket, Peer control and Peer PTY operate with the configured Public URL

#### Scenario: Access the origin directly
- **WHEN** a remote client attempts to reach the default Compose origin port on the server network
- **THEN** the host loopback publication prevents that direct network access

#### Scenario: Configure certificate files in the container
- **WHEN** operator supplies removed self-signed TLS variables or expects the Hub image to terminate TLS
- **THEN** TmuxAtlas rejects or diagnoses the unsupported configuration and points to the trusted gateway setup

### Requirement: Single-volume durable Hub state
The official Compose deployment SHALL use a single writable volume rooted at `/var/lib/tmuxatlas`, with XDG config and data subdirectories. Passkey credentials, Hub identity, Peer trust, preferences, VAPID keys and durable Push subscriptions MUST survive an image pull/recreate. The deployment MUST NOT require PostgreSQL, Redis or another external state service and MUST be documented as single-writer.

#### Scenario: Recreate the Hub container
- **WHEN** operator recreates the Hub with the same volume
- **THEN** Passkeys, Hub fingerprint, paired Peers, preferences, VAPID identity and Push subscription records remain unchanged

#### Scenario: Agents reconnect after recreate
- **WHEN** the recreated Hub starts with preserved identity and trust
- **THEN** existing Agents reconnect without new pairing and repopulate transient sessions and activity

#### Scenario: Browser Session existed before recreate
- **WHEN** a browser had a valid process-scoped Session before the Hub process was replaced
- **THEN** the old Session is rejected and the browser uses a preserved Passkey to log in again even if its configured idle TTL had not elapsed

#### Scenario: Persistent path is not writable
- **WHEN** non-root Hub cannot create or atomically replace required files in the mounted data root
- **THEN** startup or doctor returns an actionable storage/ownership failure and does not silently persist identity to ephemeral root storage

### Requirement: Local container administration
Bootstrap rotation, pairing-code generation and other same-user Hub administration SHALL remain available through the private Unix management socket from `docker compose exec`. Bootstrap plaintext MUST NOT be baked into the image, committed Compose environment or long-lived normal application logs.

#### Scenario: Obtain first-registration bootstrap
- **WHEN** no Passkey exists and operator invokes the documented container-local bootstrap command
- **THEN** the command uses the private Unix socket and returns one short-lived setup token according to the passkey-management lifecycle

#### Scenario: Generate an Agent pairing code
- **WHEN** operator invokes `docker compose exec tmuxatlas tmuxatlas pair`
- **THEN** the CLI reaches the running Hub through its local socket and returns a bounded one-time pairing code

#### Scenario: Attempt public bootstrap rotation
- **WHEN** a network client tries to invoke container administration over the public TCP router
- **THEN** no equivalent management route is exposed

### Requirement: Immutable image update and rollback
Official Docker deployments SHALL update by pulling and recreating a versioned image, not by replacing the executable inside a running container. Operational documentation MUST include health verification, volume backup, previous tag/digest rollback and an explicit warning not to remove the persistent volume.

#### Scenario: Upgrade a Docker Hub
- **WHEN** operator follows the documented update procedure
- **THEN** Compose pulls the selected image, recreates the service with the existing volume, waits for health and reports the running version

#### Scenario: New image is unhealthy
- **WHEN** the recreated service does not become healthy within the bounded window
- **THEN** operator can pin the recorded previous SemVer tag or digest and recreate with the unchanged volume

#### Scenario: Self-update is invoked in the container
- **WHEN** operator requests binary installation or rollback through `tmuxatlas update` in official Docker mode
- **THEN** the command fails closed without modifying the executable and prints the image pull/recreate and rollback workflow

### Requirement: Signed multi-architecture image release
Each stable tag release SHALL publish immutable `linux/amd64` and `linux/arm64` images to GHCR using repository-scoped GitHub Actions credentials, with version/revision OCI labels, SBOM, provenance and keyless signature. Pull-request CI MUST build and inspect the image without publishing it.

#### Scenario: Publish a release tag
- **WHEN** a valid version tag passes all release gates
- **THEN** GHCR contains matching architecture manifests and an inspectable signed digest associated with that release

#### Scenario: Build a pull request
- **WHEN** Docker changes are tested for a pull request
- **THEN** CI builds, scans/inspects and smoke-tests the image but does not authenticate for package publication or push a tag

#### Scenario: Use release credentials
- **WHEN** the release workflow publishes the image
- **THEN** it uses the repository `GITHUB_TOKEN` with minimal `packages: write` and attestation permissions and does not require a personal full-access token

### Requirement: Container integration verification
CI SHALL verify the complete supported Docker boundary: Compose rendering, no-tmux startup, role-aware health, non-root/read-only execution, graceful termination, real Agent pairing and remote terminal behavior, and persistent-volume recreation.

#### Scenario: Exercise a real Agent
- **WHEN** the container integration suite pairs a fixture Agent that owns a real tmux session
- **THEN** the browser observes the session and remote PTY input/resize through the pure Hub while the Hub container has no host tmux integration

#### Scenario: Recreate during integration
- **WHEN** the suite stops and recreates the Hub service while preserving its volume
- **THEN** durable identities and settings remain, old browser Session is rejected, and the Agent reconnects successfully

#### Scenario: Terminate an active Hub
- **WHEN** Compose sends SIGTERM while browser and Agent WebSockets are active
- **THEN** the process completes bounded graceful shutdown without corrupting the volume or leaving the service falsely healthy
