# Agent Setup

TmuxAtlas tracks AI coding agents running inside your tmux sessions. Each agent needs a hook configured so it can notify TmuxAtlas of status changes (active, waiting for input, completed, error).

The easiest way to configure all detected agents at once:

```bash
tmuxatlas agent-setup
```

Use `--dry-run` to preview changes without writing files:

```bash
tmuxatlas agent-setup --dry-run
```

## Resilience

By default, `tmuxatlas agent-setup` appends `|| true` to all hook commands so that failures (TmuxAtlas binary missing, server down, etc.) never block the agent. This means hooks are fire-and-forget — if TmuxAtlas isn't reachable, the agent continues working normally.

Additionally, `tmuxatlas notify` uses a 1-second Unix-socket timeout, so hook delivery cannot stall the agent for long.

To disable this and let hook failures propagate to the agent:

```bash
tmuxatlas agent-setup --block
```

**When to use `--block`:** If you want the agent to be aware that TmuxAtlas notifications are failing (e.g., for debugging). In normal use, the default non-blocking behavior is recommended.

**Note:** OpenCode hooks are always resilient regardless of `--block`, since the plugin uses native try/catch error handling.

## Supported Agents

### Agy

**Auto-detected; no hook is configured.** Settings shows Agy as
`auto-detected` when its executable is found. Running panes are discovered
separately by process-tree detection. Its waiting state is a TmuxAtlas silence
heuristic, not an Agy lifecycle event contract. `agent-setup` does not create
or change any Agy configuration.

### Claude Code

**Auto-configured by `tmuxatlas agent-setup`.**

TmuxAtlas adds hooks to `~/.claude/settings.json` that fire on tool use, notifications (permission prompts, input dialogs), and task completion.

**Manual setup:** Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "tmuxatlas notify -t claude -s active -m 'Using tool' || true" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "tmuxatlas notify -t claude -s active -m 'Working' || true" }]
      }
    ],
    "Notification": [
      {
        "matcher": "permission_prompt",
        "hooks": [{ "type": "command", "command": "tmuxatlas notify -t claude -s waiting -m 'Permission needed' || true" }]
      },
      {
        "matcher": "elicitation_dialog",
        "hooks": [{ "type": "command", "command": "tmuxatlas notify -t claude -s waiting -m 'Needs input' || true" }]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "tmuxatlas notify -t claude -s completed -m 'Task complete' || true" }]
      }
    ]
  }
}
```

### Codex

**Auto-configured by `tmuxatlas agent-setup`.**

Codex supports a `notify` key in `~/.codex/config.toml`. This fires when the agent needs user attention and passes the last assistant message as a JSON blob in `$1`.

**Manual setup:** Add to `~/.codex/config.toml`. The `notify` line **must appear before any table headers** (like `[sandbox]`) or Codex's TOML parser will misinterpret it:

```toml
model = "o4-mini"
notify = ["bash", "-c", "tmuxatlas notify -t codex --event-data \"$1\" || true", "--"] # tmuxatlas-agent-hook

[sandbox]
# ... rest of config
```

**How it works:**

Codex passes a JSON blob as an additional argument when the agent completes a turn and needs user input. The `--event-data` flag tells TmuxAtlas to parse this JSON natively. Fields extracted:

- `type` — event type (currently `agent-turn-complete`)
- `last-assistant-message` — truncated to 200 chars and used as the notification message
- `thread-id`, `turn-id`, `cwd` — available for context

The `agent-turn-complete` event maps to TmuxAtlas's `waiting` status, which triggers an alert in the UI and a push notification.

No external dependencies required (no bash, no jq).

### GitHub Copilot CLI

**Auto-configured by `tmuxatlas agent-setup`.**

Copilot CLI supports global hooks in `~/.copilot/hooks/` as JSON files. TmuxAtlas writes `~/.copilot/hooks/tmuxatlas.json` covering session start/end, tool use, and error events. Hooks receive event context as JSON on stdin.

**Note:** Repository-level hooks in `.github/copilot/hooks.json` take precedence over global hooks. Both run — values are concatenated across levels.

**Manual setup:** Create `~/.copilot/hooks/tmuxatlas.json`:

```json
{
  "version": 1,
  "hooks": {
    "sessionStart": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s active -m 'Session started' || true", "comment": "tmuxatlas agent hook" }],
    "sessionEnd": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s completed -m 'Session ended' || true", "comment": "tmuxatlas agent hook" }],
    "preToolUse": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s active -m 'Using tool' || true", "comment": "tmuxatlas agent hook" }],
    "postToolUse": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s active -m 'Working' || true", "comment": "tmuxatlas agent hook" }],
    "userPromptSubmitted": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s active -m 'Thinking' || true", "comment": "tmuxatlas agent hook" }],
    "errorOccurred": [{ "type": "command", "bash": "tmuxatlas notify -t copilot -s error -m 'Error occurred' || true", "comment": "tmuxatlas agent hook" }]
  }
}
```

### OpenCode

**Auto-configured by `tmuxatlas agent-setup`.**

TmuxAtlas writes a JavaScript plugin to `~/.config/opencode/plugins/tmuxatlas.js` that hooks into OpenCode's event system. The plugin registers handlers for `permission.asked`, `permission.replied`, `tool.execute.before/after`, `session.idle`, and `session.error`.

**Manual setup:** Create `~/.config/opencode/plugins/tmuxatlas.js` with a Bun-compatible plugin module that calls `tmuxatlas notify` for each event type:

| OpenCode Event | TmuxAtlas Status | Message |
|---------------|-------------|---------|
| `permission.asked` | `waiting` | "Permission needed" |
| `permission.replied` | `active` | "Working" |
| `tool.execute.before` | `active` | "Using tool" |
| `tool.execute.after` | `active` | "Working" |
| `session.idle` | `completed` | "Idle" |
| `session.error` | `error` | "Error" |

Run `tmuxatlas agent-setup` to generate the plugin file automatically.

## The `notify` command

Under the hood, all agent hooks call `tmuxatlas notify`. You can also use it directly:

```bash
# Basic usage
tmuxatlas notify -t claude -s waiting -m "Needs approval"
tmuxatlas notify -t codex -s active
tmuxatlas notify -t claude -s completed

# Read event JSON from stdin (used by some agent hooks)
echo '{"hook_event_name":"Stop","last_assistant_message":"Done"}' | tmuxatlas notify -t codex --stdin
```

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--tool` | `-t` | Agent name: `claude`, `codex`, `copilot`, `opencode`, `agy` |
| `--status` | `-s` | Status: `active`, `waiting`, `completed`, `error` |
| `--message` | `-m` | Human-readable message |
| `--stdin` | | Read hook event JSON from stdin |
| `--session` | | tmux session name (auto-detected) |
| `--window` | | tmux window index (auto-detected) |
| `--pane` | | tmux pane ID (auto-detected) |
| `--server` | | TmuxAtlas server URL (default: `http://localhost:7654`) |
| `--socket` | | Unix socket path (auto-detected) |

**Communication:** `tmuxatlas notify` uses only the current user's private Unix socket. It never falls back to an unauthenticated HTTP endpoint. Run either the local Hub or Agent service on every machine that emits hook events.

## Inactivity-based waiting detection

Claude Code sends explicit "waiting" events when it needs input (permission prompts, input dialogs). Other tools (Copilot, Codex, OpenCode) don't have equivalent hooks.

To bridge this gap, TmuxAtlas includes an **inactivity promoter**: if a non-Claude tool sends "active" events but then goes quiet for 30 seconds, TmuxAtlas automatically generates a synthetic "waiting" event with the message "May need attention". This surfaces the alert in the UI and triggers push notifications, just like a native waiting event would.

This only applies to tools without native waiting support — Claude's explicit hooks are always trusted and never overridden.

The timeout is 30 seconds by default. This balances catching idle agents quickly against false positives during normal pauses between tool calls.

## Status values

| Status | Meaning | UI behavior |
|--------|---------|-------------|
| `active` | Agent is working normally | Badge in sidebar |
| `waiting` | Agent needs user input | Alert banner + push notification |
| `error` | Agent hit an error | Alert banner + push notification |
| `completed` | Agent finished its task | Clears alerts |
