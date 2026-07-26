package agentcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AgentStatus represents the installation and configuration state of a single agent.
type AgentStatus struct {
	Name       string `json:"name"`
	Key        string `json:"key"`
	Installed  bool   `json:"installed"`
	Configured bool   `json:"configured"`
}

// StatusResult contains the status of all known agents and the setup command.
type StatusResult struct {
	Agents       []AgentStatus `json:"agents"`
	SetupCommand string        `json:"setup_command"`
}

// CheckAgents checks which agents are installed and whether their tmuxatlas hooks are configured.
func CheckAgents() *StatusResult {
	home, _ := os.UserHomeDir()

	result := &StatusResult{
		SetupCommand: "tmuxatlas agent-setup",
		Agents: []AgentStatus{
			{
				Name:       "Claude Code",
				Key:        "claude",
				Installed:  isInstalled("claude"),
				Configured: isClaudeConfigured(home),
			},
			{
				Name:       "Codex",
				Key:        "codex",
				Installed:  isInstalled("codex"),
				Configured: isCodexConfigured(home),
			},
			{
				Name:       "Copilot",
				Key:        "copilot",
				Installed:  isInstalled("copilot"),
				Configured: isCopilotConfigured(home),
			},
			{
				Name:       "OpenCode",
				Key:        "opencode",
				Installed:  isInstalled("opencode"),
				Configured: isOpenCodeConfigured(home),
			},
		},
	}
	return result
}

func isInstalled(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
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
