# Agent Support Matrix

## TmuxAtlas Hub/Agent runtime compatibility

| Hub runtime | Agent runtime | State sync | Session actions | Remote PTY |
|---|---|---|---|---|
| v1 | v1 with negotiated capability | Yes | Yes | Yes |
| v1 | v1 without optional capability | Yes | Explicit `capability-unsupported` | Explicit `capability-unsupported` |
| v1 | legacy/no hello | No | No | No |
| legacy | v1-only Agent | Unsupported mixed-version deployment | No | No |

Upgrade the Hub first and all Agents immediately afterward. During that window,
legacy Agents are deliberately offline. Use the Hub `/api/hosts` response to
check build version, `runtime_protocol`, `generation`, and `capabilities`.
Hub and Agents must be rolled back as a matching set.

## Support Tiers

| Agent | Support Level | Detection | Waiting Detection | Latency |
|-------|--------------|-----------|-------------------|---------|
| **Claude Code** | **Full (Native)** | Hook-based | Native hooks | <1-2s |
| **OpenCode** | **Full (Native)** | Plugin-based hooks | Native hooks | <1-2s |
| **GitHub Copilot CLI** | **Partial (Hybrid)** | Hooks + process tree | Silence monitor + inactivity promoter | 10-30s |
| **Codex** | **Partial (Hybrid)** | Hooks + process tree | Silence monitor + inactivity promoter | 10-30s |
| **Agy** | **Passive (Heuristic)** | Process tree | Silence monitor | 10-20s |

## Feature Breakdown

| Feature | Claude | OpenCode | Copilot | Codex | Agy |
|---------|--------|----------|---------|-------|-----|
| Auto-detection (process tree) | Yes | Yes | Yes | Yes | Yes |
| Hook-based activity tracking | Yes | Yes | Yes | Limited (turn-complete only) | No |
| Native waiting state | Yes | Yes | No | No | No |
| Tool use events | Yes | Yes | Yes | No | No |
| Thinking/working events | Yes | Yes | Yes | No | Heuristic |
| Permission prompt detection | Native | Native | Silence-based (~10s) | Silence-based (~10s) | Silence-based (~10s) |
| Completion events | Native | Native | Native | Native | Detector-owned |
| Error events | No | Native | Native | No | No |
| Push notifications on waiting | Instant | Instant | Delayed (10-30s) | Delayed (10-30s) | Delayed (10-20s) |
| Auto setup (`tmuxatlas agent-setup`) | Yes | Yes | Yes | Yes | No |

## Detection Layers

1. **Hook-based** (<1-2s) — Agent calls `tmuxatlas notify` via configured hooks, which spawns a CLI process, delivers via Unix socket, and broadcasts over WebSocket. Claude and OpenCode get full granular events including explicit "waiting" states.
2. **Process tree scanning** (~5s) — Scans tmux panes every 5s, reads `/proc/<pid>/cmdline` to detect agent processes. Works for all agents as a fallback.
3. **Silence monitor** (~10-20s) — For non-native-waiting agents. After 10s of silence, runs `tmux capture-pane` and checks for approval prompts (`(y/n)`, `allow/deny`, etc.). Limited to 2 checks per silence period.
4. **Inactivity promoter** (~30s) — Last-resort fallback. If no activity for 30s, generates synthetic "waiting" event for Copilot/Codex.
5. **Reconciler** (~3s) — Cleans up stale events when agent process exits (foreground returns to shell).

## Why Claude Has Best Support

Claude Code has native hook support for every lifecycle event — `PreToolUse`, `PostToolUse`, `Notification` (permission prompts, input dialogs), and `Stop`. This means TmuxAtlas knows within seconds when Claude is thinking, using a tool, waiting for permission, or done. No polling, no guessing.

## Why OpenCode Is Close Behind

OpenCode has a JavaScript plugin system that fires events for `permission.asked`, `permission.replied`, `tool.execute.before/after`, `session.idle`, and `session.error`. This gives comparable instant tracking to Claude.

## How We Support Other Agents

Copilot and Codex lack native "waiting" hooks, so TmuxAtlas uses a hybrid approach:

- **Copilot** has hooks for `sessionStart/End`, `preToolUse`, `postToolUse`, and `userPromptSubmitted` — good activity tracking, but waiting detection relies on silence monitoring and capture-pane prompt parsing.
- **Codex** only fires a single `agent-turn-complete` hook, so it has the least granular hook coverage. Waiting and activity detection rely heavily on process tree scanning and silence monitoring.

For both, the silence monitor parses captured pane content looking for interactive prompt patterns, and the inactivity promoter generates synthetic waiting events after 30s of no activity.

## Agy Passive Support

Agy is intentionally process-detected only. TmuxAtlas writes no Agy plugin or
configuration and does not claim a native Agy event contract. A detected Agy
pane can receive a heuristic waiting state from the silence monitor; resumed
output restores the synthetic active state, and detector-observed departure
clears the Agy projection without clearing a replacement agent's state on the
same pane.
