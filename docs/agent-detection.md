# Agent Detection & Event Tracking

TmuxAtlas uses a multi-layered approach to detect AI coding agents running in tmux panes and track their state. This document describes how each agent is detected, what events they produce, and how "waiting for input" state is determined.

## Detection Layers

There are five layers, from most precise to most general:

| Layer | Mechanism | Latency | Accuracy |
|-------|-----------|---------|----------|
| **Hook-based** | Agent calls `tmuxatlas notify` via configured hooks | Instant | Exact |
| **Process tree** | Scan `/proc` for known agent binaries | ~5s | High |
| **Silence + capture-pane** | Detect quiet panes, inspect content for prompts | ~10-20s | Medium |
| **Inactivity promoter** | Promote to "waiting" after 30s of no hook activity | ~30s | Low |
| **Reconciler** | Clear stale events when agent process exits | ~3s | High |

Each layer fills gaps left by the ones above it. An agent with hooks configured gets instant, accurate tracking. An agent with no hooks still gets detected via process tree scanning, with prompt detection via pane content inspection.

## Per-Agent Breakdown

### Claude Code

**Detection:** Hook-based (native waiting).

**Hooks configured** (`~/.claude/settings.json`):
- `PreToolUse` → `active` ("Using tool")
- `PostToolUse` → `active` ("Working")
- `Notification` (permission_prompt) → `waiting` ("Permission needed")
- `Notification` (elicitation_dialog) → `waiting` ("Needs input")
- `Stop` → `completed` ("Task complete")

**Waiting detection:** Native — Claude sends explicit `waiting` events when it needs user approval or input. No fallback detection is used.

**Process tree match:** Binary named `claude`.

### Codex

**Detection:** Hybrid — hook for completion, process tree + silence monitor for waiting.

**Hook configured** (`~/.codex/config.toml`):
- `notify = ["tmuxatlas", "notify", "-t", "codex", "--event-data"]`
- Fires on `agent-turn-complete` → `completed` status
- Message extracted from `last-assistant-message` field (truncated to 200 chars)

**Waiting detection:** Silence monitor. When codex goes quiet for 10s, TmuxAtlas captures the pane content and looks for approval prompts (e.g., `allow / deny`, `(y/n)`). Limited to 2 checks per silence period to avoid unnecessary tmux commands.

**Process tree match:** Binary named `codex`, or node script with "codex" in the path (e.g., `node /usr/lib/node_modules/@openai/codex/bin/codex.js`).

### GitHub Copilot CLI

**Detection:** Hook-based for activity, silence monitor for waiting.

**Hooks configured** (`~/.copilot/hooks/tmuxatlas.json`):
- `sessionStart` → `active` ("Session started")
- `sessionEnd` → `completed` ("Session ended")
- `preToolUse` → `active` ("Using tool")
- `postToolUse` → `active` ("Working")
- `userPromptSubmitted` → `active` ("Thinking")
- `errorOccurred` → `error` ("Error occurred")

**Waiting detection:** Two mechanisms:
1. **Inactivity promoter** — if hooks are firing but then go quiet for 30s, promotes to `waiting` ("May need attention")
2. **Silence monitor** — if detected via process tree (no hooks), captures pane content after 10s of silence

**Note:** Repository-level hooks (`.github/copilot/hooks.json`) take precedence over global hooks and can override them.

**Process tree match:** Binary named `copilot`, or node script with "copilot" in the path.

### OpenCode

**Detection:** Hook-based (native waiting) via plugin system.

**Plugin configured** (`~/.config/opencode/plugins/tmuxatlas.js`):
- `permission.asked` → `waiting` ("Permission needed")
- `permission.replied` → `active` ("Working")
- `tool.execute.before` → `active` ("Using tool")
- `tool.execute.after` → `active` ("Working")
- `session.idle` → `completed` ("Idle")
- `session.error` → `error` ("Error")

**Waiting detection:** Native — OpenCode sends explicit `waiting` events via the `permission.asked` hook when it needs user approval. Like Claude, no fallback silence/inactivity detection is needed.

**Process tree match:** Binary named `opencode`.

### Agy

**Detection:** Passive process-tree detection. Agy has no generated TmuxAtlas
plugin or hook because no stable lifecycle callback contract is assumed.

**Waiting detection:** Heuristic silence and prompt detection. Resumed pane
output clears a synthetic waiting state while the process is still detected.

**Process tree match:** Exact executable basename `agy`.

## How Each Layer Works

### Hook-Based Detection

Agents call `tmuxatlas notify` which sends an event only through the current user's private Unix socket. The notify command auto-detects the tmux session, window, and pane from the `TMUX_PANE` environment variable.

Event delivery path:
```
Agent hook → tmuxatlas notify → unix socket → POST /api/tool-event → Tracker.Record() → WebSocket broadcast
```

The server stamps the local host identity on incoming events for multi-host navigation.

### Process Tree Detection

The detector (`pkg/toolevents/detector.go`) runs every 5 seconds:
1. Lists all tmux panes via `list-panes -a`
2. For each pane, reads `/proc/<pid>/cmdline` for child processes
3. Checks children and grandchildren against known agent patterns (handles `shell → node → agent` chains)
4. Records synthetic `active` events with `auto_detected: true`
5. Skips panes that already have hook-based tracking

### Silence Monitor + Capture-Pane

The silence monitor (`pkg/toolevents/silence.go`) watches detected non-Claude
panes. It supplies waiting-state detection for agents without native waiting
hooks (currently Codex, Copilot, and passive Agy); agents with native hooks use
their hook events when available:
1. Tracks last output time per pane via `%output` control mode notifications
2. When a pane has been quiet for 10+ seconds, runs `tmux capture-pane -p` to get visible text
3. Passes the last ~10 non-empty lines to `DetectPrompt()` which checks for:
   - **Approval patterns:** `(y/n)`, `[Y/n]`, `approve`, `deny`, `yes/no`
   - **Numbered option lists** followed by a prompt character (`>`, `?`, `:`)
   - **Agent-specific patterns:** codex `allow/deny`, copilot selection menus, `press enter`
   - **Input prompts:** short lines ending with `?`, `>`, or `:` (excludes shell prompts like `❯`, `$`, `#`, `%`, `user@host:~$`)
4. If a prompt is found, records a `waiting` event
5. Checks are limited to 2 per silence period — resets when output resumes

### Inactivity Promoter

For tools without native waiting hooks, the tracker watches for prolonged silence after hook-based `active` events:
- If no new event arrives within 30 seconds, promotes to `waiting` ("May need attention")
- Only applies to hook-based activity (not auto-detected events, since those can't distinguish working vs idle)
- Acts as a fallback when silence monitor doesn't trigger (e.g., control mode not running)

### Reconciler

Runs every 3 seconds to clean up stale events:
- Checks if the pane still exists
- Checks if the foreground process is a shell (zsh, bash, fish, etc.) — meaning the agent exited
- If either condition is true, clears the event with `completed` status

## Event Lifecycle

```
Agent starts in pane
    │
    ├─ Hook fires → active event recorded
    │   └─ Hook fires again → active event (clears previous)
    │       └─ Hook fires waiting → waiting event (alert shown)
    │           └─ Hook fires active → clears waiting (alert dismissed)
    │
    ├─ No hooks → Detector finds process → synthetic active (auto_detected)
    │   └─ Output stops for 10s → Silence monitor captures pane
    │       ├─ Prompt found → waiting event (alert shown)
    │       │   └─ Output resumes → checks reset, reconciler clears if agent exits
    │       └─ No prompt → checks exhausted (max 2), waits for output to resume
    │
    └─ Agent exits
        └─ Reconciler detects shell foreground → completed event (alert cleared)
```

## Multi-Host Considerations

In hub/peer topologies, events need the host identity for correct frontend navigation. The server stamps `Host` and `HostName` on events received via the HTTP API. Components that record events directly (detector, silence monitor) have their host identity set at startup via `SetHost()`.

Events without host info will navigate to local sessions only, which breaks in multi-host setups where the session might be on a remote peer.
