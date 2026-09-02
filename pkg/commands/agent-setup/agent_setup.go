package agentsetup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/agentcheck"
	"github.com/LosFurina/tmuxatlas/pkg/common"
)

type agentConfig struct {
	name          string
	key           string
	binary        string
	detected      bool
	setupRequired bool
	setup         func(tmuxatlasBin string, resilient bool, extraDirs []string) error
}

func Execute(ctx context.Context, c *cli.Command) error {
	// Parse --config-dir flags into map[agentKey][]string
	extraDirs := make(map[string][]string)
	for _, val := range c.StringSlice("config-dir") {
		parts := strings.SplitN(val, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid --config-dir format %q, expected agent=path", val)
		}
		extraDirs[parts[0]] = append(extraDirs[parts[0]], parts[1])
	}

	// Find tmuxatlas binary path
	tmuxatlasBin, err := os.Executable()
	if err != nil {
		tmuxatlasBin = "tmuxatlas"
	}

	// If running `go run`, use the binary name directly
	if strings.Contains(tmuxatlasBin, "go-build") {
		tmuxatlasBin = "tmuxatlas"
	}

	agents := []agentConfig{
		{
			name:          "Claude Code",
			key:           "claude",
			binary:        "claude",
			setup:         setupClaude,
			setupRequired: true,
		},
		{
			name:          "Codex",
			key:           "codex",
			binary:        "codex",
			setup:         setupCodex,
			setupRequired: true,
		},
		{
			name:          "GitHub Copilot CLI",
			key:           "copilot",
			binary:        "copilot",
			setup:         setupCopilot,
			setupRequired: true,
		},
		{
			name:          "OpenCode",
			key:           "opencode",
			binary:        "opencode",
			setup:         setupOpenCode,
			setupRequired: true,
		},
		{
			name: "Agy", key: "agy", binary: "agy", setupRequired: false,
		},
	}

	// Detect installed agents
	fmt.Println("Detecting installed AI agents...")
	fmt.Println()
	for i := range agents {
		home, _ := os.UserHomeDir()
		agents[i].detected = detectInstalled(agents[i].binary, home)
		status := "not found"
		if agents[i].detected {
			status = "found"
		}
		fmt.Printf("  %-20s %s\n", agents[i].name, status)
	}
	fmt.Println()

	dryRun := c.Bool("dry-run")
	resilient := !c.Bool("block")

	anySetup := false
	for _, agent := range agents {
		if !agent.detected {
			continue
		}
		if !agent.setupRequired {
			fmt.Printf("Agy is auto-detected; no hook configuration is written.\n")
			continue
		}
		anySetup = true
		extras := extraDirs[agent.key]
		if dryRun {
			fmt.Printf("Would configure hooks for %s\n", agent.name)
			for _, dir := range extras {
				fmt.Printf("  Would also configure: %s\n", dir)
			}
		} else {
			fmt.Printf("Configuring hooks for %s...\n", agent.name)
			if err := agent.setup(tmuxatlasBin, resilient, extras); err != nil {
				logrus.WithError(err).WithField("agent", agent.name).Warn("failed to configure")
				fmt.Printf("  Warning: %v\n", err)
			} else {
				fmt.Printf("  Done.\n")
			}
		}
	}

	if !anySetup {
		fmt.Println("No hook-configurable agents found. Install one of: claude, codex, gh (copilot), opencode")
		return nil
	}

	fmt.Println()
	fmt.Println("Agent hooks configured. They will notify TmuxAtlas through the local Unix socket.")
	return nil
}

func detectInstalled(binary, home string) bool {
	_, ok := agentcheck.FindExecutable(binary, home)
	return ok
}

// setupClaude configures Claude Code hooks in ~/.claude/settings.json and any extra dirs
func setupClaude(tmuxatlasBin string, resilient bool, extraDirs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := append([]string{filepath.Join(homeDir, ".claude")}, extraDirs...)
	for _, dir := range dirs {
		if err := setupClaudeDir(dir, tmuxatlasBin, resilient); err != nil {
			return err
		}
	}
	return nil
}

func setupClaudeDir(configDir, tmuxatlasBin string, resilient bool) error {
	settingsPath := filepath.Join(configDir, "settings.json")

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	}

	// Notify auto-discovers the unix socket, so no --server needed
	notifyCmd := fmt.Sprintf("%s notify", tmuxatlasBin)

	suffix := ""
	if resilient {
		suffix = " || true"
	}

	// Build hooks config — each hook must be {"type": "command", "command": "..."}
	makeHook := func(cmd string) []map[string]interface{} {
		return []map[string]interface{}{
			{"type": "command", "command": cmd + suffix},
		}
	}

	hooks := map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{
				"matcher": "",
				"hooks":   makeHook(notifyCmd + " -t claude -s active -m 'Using tool'"),
			},
		},
		"PostToolUse": []map[string]interface{}{
			{
				"matcher": "",
				"hooks":   makeHook(notifyCmd + " -t claude -s active -m 'Working'"),
			},
		},
		"Notification": []map[string]interface{}{
			{
				"matcher": "permission_prompt",
				"hooks":   makeHook(notifyCmd + " -t claude -s waiting -m 'Permission needed'"),
			},
			{
				"matcher": "elicitation_dialog",
				"hooks":   makeHook(notifyCmd + " -t claude -s waiting -m 'Needs input'"),
			},
		},
		"Stop": []map[string]interface{}{
			{
				"matcher": "",
				"hooks":   makeHook(notifyCmd + " -t claude -s completed -m 'Task complete'"),
			},
		},
	}

	settings["hooks"] = hooks

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return err
	}

	fmt.Printf("  Wrote hooks to %s\n", settingsPath)
	return nil
}

// setupCodex configures Codex CLI via ~/.codex/config.toml and any extra dirs
func setupCodex(tmuxatlasBin string, resilient bool, extraDirs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := append([]string{filepath.Join(homeDir, ".codex")}, extraDirs...)
	for _, dir := range dirs {
		if err := setupCodexDir(dir, tmuxatlasBin, resilient); err != nil {
			return err
		}
	}
	return nil
}

// setupCodexDir configures a single Codex config directory.
// Codex supports a `notify` key in config.toml that fires when the agent
// needs user attention. The value is an argv array passed to execvp.
func setupCodexDir(configDir, tmuxatlasBin string, resilient bool) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.toml")

	// Codex passes the event JSON as argv[1] to the notify command.
	// tmuxatlas notify --event-data parses it natively — no bash/jq needed.
	var notifyLine string
	if resilient {
		// Wrap in bash to support || true — Codex appends event JSON as $1
		notifyLine = fmt.Sprintf(
			`notify = ["bash", "-c", "%s notify -t codex --event-data \"$1\" || true", "--"] # tmuxatlas-agent-hook`,
			tmuxatlasBin,
		)
	} else {
		notifyLine = fmt.Sprintf(
			`notify = ["%s", "notify", "-t", "codex", "--event-data"] # tmuxatlas-agent-hook`,
			tmuxatlasBin,
		)
	}

	// Read existing config.toml and update/insert the notify line.
	// IMPORTANT: notify must appear as a top-level key BEFORE any TOML
	// table headers (e.g. [sandbox]). Codex's TOML parser treats keys
	// after a table header as belonging to that table, so we insert
	// right after the model line (or other top-level keys at the start).
	var lines []string
	data, err := os.ReadFile(configPath)
	if err == nil {
		lines = strings.Split(string(data), "\n")
	}

	// First pass: remove any existing notify line
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "notify") && strings.Contains(trimmed, "=") {
			continue
		}
		filtered = append(filtered, line)
	}
	lines = filtered

	// Second pass: insert notify after the last top-level key-value line
	// and before the first table header ([section]).
	insertIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			// Hit a table header — insert before it
			break
		}
		// It's a top-level key=value line; insert after it
		insertIdx = i + 1
	}

	// Insert the notify line
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, notifyLine)
	newLines = append(newLines, lines[insertIdx:]...)
	lines = newLines

	content := strings.Join(lines, "\n")
	// Ensure file ends with a newline
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return err
	}

	fmt.Printf("  Updated %s\n", configPath)
	return nil
}

// setupCopilot configures GitHub Copilot CLI hooks via ~/.copilot/hooks/tmuxatlas.json and any extra dirs
func setupCopilot(tmuxatlasBin string, resilient bool, extraDirs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := append([]string{filepath.Join(homeDir, ".copilot")}, extraDirs...)
	for _, dir := range dirs {
		if err := setupCopilotDir(dir, tmuxatlasBin, resilient); err != nil {
			return err
		}
	}
	return nil
}

// setupCopilotDir configures a single Copilot config directory.
// Copilot CLI supports global hooks in hooks/ as JSON files.
func setupCopilotDir(configDir, tmuxatlasBin string, resilient bool) error {
	hooksDir := filepath.Join(configDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	hooksPath := filepath.Join(hooksDir, "tmuxatlas.json")
	notifyCmd := fmt.Sprintf("%s notify", tmuxatlasBin)

	suffix := ""
	if resilient {
		suffix = " || true"
	}

	makeHook := func(status, message string) map[string]interface{} {
		return map[string]interface{}{
			"type":    "command",
			"bash":    fmt.Sprintf("%s -t copilot -s %s -m '%s'%s", notifyCmd, status, message, suffix),
			"comment": "tmuxatlas agent hook",
		}
	}

	hooks := map[string]interface{}{
		"version": 1,
		"hooks": map[string]interface{}{
			"sessionStart":        []map[string]interface{}{makeHook("active", "Session started")},
			"sessionEnd":          []map[string]interface{}{makeHook("completed", "Session ended")},
			"preToolUse":          []map[string]interface{}{makeHook("active", "Using tool")},
			"postToolUse":         []map[string]interface{}{makeHook("active", "Working")},
			"userPromptSubmitted": []map[string]interface{}{makeHook("active", "Thinking")},
			"errorOccurred":       []map[string]interface{}{makeHook("error", "Error occurred")},
		},
	}

	out, err := json.MarshalIndent(hooks, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(hooksPath, out, 0o644); err != nil {
		return err
	}

	fmt.Printf("  Wrote hooks to %s\n", hooksPath)
	removeLegacyFile(filepath.Join(hooksDir, "guppi.json"))
	return nil
}

// setupOpenCode configures OpenCode via native plugin in ~/.config/opencode and any extra dirs
func setupOpenCode(tmuxatlasBin string, resilient bool, extraDirs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := append([]string{filepath.Join(homeDir, ".config", "opencode")}, extraDirs...)
	for _, dir := range dirs {
		if err := setupOpenCodeDir(dir, tmuxatlasBin); err != nil {
			return err
		}
	}
	return nil
}

func setupOpenCodeDir(configDir, tmuxatlasBin string) error {
	pluginsDir := filepath.Join(configDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return err
	}

	// Bun's $ uses backtick template literals which conflict with Go raw strings,
	// so we build the JS content via string concatenation.
	quotedBin := fmt.Sprintf("%q", tmuxatlasBin)
	plugin := "export const TmuxAtlasPlugin = async ({ $ }) => {\n" +
		"  const tmuxatlas = " + quotedBin + ";\n" +
		"\n" +
		"  const notify = async (status, message) => {\n" +
		"    try {\n" +
		"      await $`${tmuxatlas} notify -t opencode -s ${status} -m ${message}`.quiet();\n" +
		"    } catch (e) {\n" +
		"      // Silent - monitoring should never crash the host tool\n" +
		"    }\n" +
		"  };\n" +
		"\n" +
		"  return {\n" +
		`    "permission.asked": async () => { await notify("waiting", "Permission needed"); },` + "\n" +
		`    "permission.replied": async () => { await notify("active", "Working"); },` + "\n" +
		`    "tool.execute.before": async () => { await notify("active", "Using tool"); },` + "\n" +
		`    "tool.execute.after": async () => { await notify("active", "Working"); },` + "\n" +
		`    "session.idle": async () => { await notify("completed", "Idle"); },` + "\n" +
		`    "session.error": async () => { await notify("error", "Error"); },` + "\n" +
		"  };\n" +
		"};\n"

	pluginFile := filepath.Join(pluginsDir, "tmuxatlas.js")
	if err := os.WriteFile(pluginFile, []byte(plugin), 0o644); err != nil {
		return err
	}

	fmt.Printf("  Wrote plugin to %s\n", pluginFile)

	// Prevent duplicate events from pre-rename plugins and older hook scripts.
	removeLegacyFile(filepath.Join(pluginsDir, "guppi.js"))
	removeLegacyFile(filepath.Join(configDir, "guppi-hook.sh"))
	removeLegacyFile(filepath.Join(configDir, "tmuxatlas-hook.sh"))

	return nil
}

func removeLegacyFile(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := os.Remove(path); err != nil {
		fmt.Printf("  Warning: could not remove legacy hook %s: %v\n", path, err)
		return
	}
	fmt.Printf("  Removed legacy hook %s\n", path)
}

func init() {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "show what would be configured without making changes",
		},
		&cli.BoolFlag{
			Name:  "block",
			Usage: "allow hook failures to block agents (by default hooks append '|| true' so failures are ignored)",
		},
		&cli.StringSliceFlag{
			Name:  "config-dir",
			Usage: "additional config directory for an agent (format: agent=path, repeatable)",
		},
	}

	cmd := &cli.Command{
		Name:  "agent-setup",
		Usage: "configure AI agent hooks to notify TmuxAtlas",
		Description: `Detects installed AI coding tools and configures their hooks
to send status notifications through the local TmuxAtlas Unix socket.

Supported agents:
  - Claude Code (claude)
  - Codex (codex)
  - GitHub Copilot CLI (gh copilot)
  - OpenCode (opencode)
  - Agy (agy, auto-detected; no hook is written)

Use --dry-run to preview changes without writing files.`,
		Flags:  flags,
		Action: Execute,
	}

	common.RegisterCommand(cmd)
}
