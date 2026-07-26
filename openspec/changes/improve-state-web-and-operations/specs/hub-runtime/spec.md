## ADDED Requirements

### Requirement: Explicit runtime roles
TmuxAtlas SHALL expose explicit pure Hub, outbound Agent and standalone runtime compositions. `tmuxatlas hub` MUST run the pure Hub composition, `tmuxatlas agent` MUST remain outbound-only, and `tmuxatlas standalone` MUST compose Hub with local tmux integration. The existing `tmuxatlas server` command SHALL remain a compatibility entry for standalone behavior and MUST NOT silently change into pure Hub behavior.

#### Scenario: Start a pure Hub
- **WHEN** operator runs `tmuxatlas hub` on a machine or image without tmux
- **THEN** TmuxAtlas starts its Hub services without looking up, connecting to or executing tmux

#### Scenario: Start an outbound Agent
- **WHEN** operator runs `tmuxatlas agent` with a paired Hub URL
- **THEN** TmuxAtlas starts local tmux integration and the outbound Peer connection without opening a public TCP listener

#### Scenario: Preserve the server command
- **WHEN** an existing installation starts `tmuxatlas server`
- **THEN** TmuxAtlas retains the standalone Hub-plus-local-tmux composition expected by that installation

### Requirement: Pure Hub local-integration isolation
The pure Hub composition SHALL provide Web, Passkey, preferences, Push, Peer registry/control, pairing, canonical state, remote PTY relay and local administration without constructing a tmux client, discovery loop, control-mode client, process-tree detector, silence monitor, local activity/stats producer or local PTY executor. Optional local integration MUST be supplied through explicit composition interfaces rather than implicit nil clients.

#### Scenario: Run with no tmux executable
- **WHEN** the pure Hub process starts with no tmux binary or socket available
- **THEN** startup and readiness succeed without degraded tmux warnings or retry loops

#### Scenario: Inspect the pure Hub process
- **WHEN** role-matrix tests observe the pure Hub dependencies and background loops
- **THEN** no local tmux, process-tree, detector, discovery, activity or PTY-executor dependency has been constructed or started

#### Scenario: Run standalone integration
- **WHEN** the standalone role starts on a machine with tmux
- **THEN** the same Hub core receives the explicitly composed local producer and executor without changing remote Agent behavior

### Requirement: Remote-only session behavior in pure Hub
A pure Hub SHALL derive tmux session facts and executable targets only from authenticated, active Agents. It MUST NOT synthesize a local tmux host or fall back to the container/Hub machine when a target is missing, offline or unsupported.

#### Scenario: No Agents are connected
- **WHEN** a pure Hub has no active Agents
- **THEN** its dashboard and administration remain available, its session projection is empty, and the process remains ready

#### Scenario: Target an active Agent
- **WHEN** a browser action or PTY request names a valid session target owned by an active Agent
- **THEN** the Hub routes it through that Agent's negotiated runtime connection

#### Scenario: Target the Hub as a tmux host
- **WHEN** a caller addresses the pure Hub identity as though it owned a local tmux session
- **THEN** TmuxAtlas returns a structured unsupported or not-found outcome and executes no local command

#### Scenario: Remote target becomes unavailable
- **WHEN** an Agent is offline or its target is stale
- **THEN** the pure Hub returns the applicable Peer/target error and does not select another Agent or local fallback

### Requirement: Role-aware installation, diagnostics and readiness
Service installation, `doctor`, local health responses and Fleet facts SHALL use an explicit runtime role. Pure Hub readiness MUST mean that required Hub core loops and local administration are ready; it MUST NOT require a Passkey to have been registered or an Agent to be online.

#### Scenario: Install a native Hub service
- **WHEN** operator selects `--mode hub`
- **THEN** systemd or launchd installs a service that runs `tmuxatlas hub` and records the Hub role

#### Scenario: Diagnose a pure Hub
- **WHEN** `tmuxatlas doctor` runs for a healthy pure Hub with no tmux installed
- **THEN** tmux absence is not reported as an error and Hub-specific listener, storage, Public URL and health checks are evaluated

#### Scenario: Hub awaits first Passkey
- **WHEN** Hub core is ready but first-registration setup is still required
- **THEN** local health reports `role=hub` and process ready while separately exposing a non-secret setup-required fact

#### Scenario: Agent health is evaluated
- **WHEN** the same health contract is queried from an Agent
- **THEN** it reports `role=agent` and evaluates its local socket and outbound runtime rather than Hub HTTP readiness

### Requirement: Graceful Hub lifecycle
Pure Hub and standalone runtimes SHALL handle termination with a bounded, observable shutdown. They MUST stop accepting new browser/Peer/PTY work, cancel owned loops, close active transports and the local management socket, and exit within the configured grace period.

#### Scenario: Container receives SIGTERM
- **WHEN** a pure Hub receives SIGTERM while browser and Agent connections exist
- **THEN** it stops new work, closes or drains owned connections, flushes required file state and exits before the container stop timeout

#### Scenario: Shutdown runs more than once
- **WHEN** cancellation, server error and a second termination signal race
- **THEN** lifecycle teardown remains idempotent and does not panic, deadlock or leak the local socket

#### Scenario: Agent reconnects after Hub restart
- **WHEN** a pure Hub restarts with preserved identity and Peer trust
- **THEN** paired Agents reconnect through their normal backoff and rebuild transient Hub state without re-pairing
