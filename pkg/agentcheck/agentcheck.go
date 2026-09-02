package agentcheck

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentStatus represents the installation and configuration state of a single agent.
type AgentStatus struct {
	Name          string `json:"name"`
	Key           string `json:"key"`
	Installed     bool   `json:"installed"`
	Configured    bool   `json:"configured"`
	TrackingMode  string `json:"tracking_mode,omitempty"`
	SetupRequired bool   `json:"setup_required"`
}

// StatusResult contains the status of all known agents and the setup command.
type StatusResult struct {
	Agents       []AgentStatus `json:"agents"`
	SetupCommand string        `json:"setup_command"`
}

// CheckAgents checks which agents are installed and whether their tmuxatlas hooks are configured.
func CheckAgents() *StatusResult {
	home, _ := os.UserHomeDir()
	return checkAgents(home)
}

func checkAgents(home string) *StatusResult {

	result := &StatusResult{
		SetupCommand: "tmuxatlas agent-setup",
		Agents: []AgentStatus{
			{
				Name:         "Claude Code",
				Key:          "claude",
				Installed:    isInstalled("claude", home),
				Configured:   isClaudeConfigured(home),
				TrackingMode: "hook", SetupRequired: true,
			},
			{
				Name:         "Codex",
				Key:          "codex",
				Installed:    isInstalled("codex", home),
				Configured:   isCodexConfigured(home),
				TrackingMode: "hook", SetupRequired: true,
			},
			{
				Name:         "Copilot",
				Key:          "copilot",
				Installed:    isInstalled("copilot", home),
				Configured:   isCopilotConfigured(home),
				TrackingMode: "hook", SetupRequired: true,
			},
			{
				Name:         "OpenCode",
				Key:          "opencode",
				Installed:    isInstalled("opencode", home),
				Configured:   isOpenCodeConfigured(home),
				TrackingMode: "hook", SetupRequired: true,
			},
			{
				Name: "Agy", Key: "agy", Installed: isInstalled("agy", home),
				Configured: false, TrackingMode: "passive", SetupRequired: false,
			},
		},
	}
	return result
}

func isInstalled(binary, home string) bool {
	_, ok := FindExecutable(binary, home)
	return ok
}

// FindExecutable returns an executable regular file from PATH or conservative
// user-owned installation directories. Candidates are never executed.
func FindExecutable(binary, home string) (string, bool) {
	if binary == "" || filepath.Base(binary) != binary {
		return "", false
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if path := executableFile(filepath.Join(directory, binary)); path != "" {
			return path, true
		}
	}
	for _, directory := range []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin")} {
		if path := executableFile(filepath.Join(directory, binary)); path != "" {
			return path, true
		}
	}
	versions, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin"))
	for _, directory := range versions {
		if path := executableFile(filepath.Join(directory, binary)); path != "" {
			return path, true
		}
	}
	return "", false
}

func executableFile(path string) string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return path
}

func isClaudeConfigured(home string) bool {
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "tmuxatlas")
}

func isCodexConfigured(home string) bool {
	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "tmuxatlas")
}

func isCopilotConfigured(home string) bool {
	hookPath := filepath.Join(home, ".copilot", "hooks", "tmuxatlas.json")
	_, err := os.Stat(hookPath)
	return err == nil
}

func isOpenCodeConfigured(home string) bool {
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "tmuxatlas.js")
	_, err := os.Stat(pluginPath)
	return err == nil
}
