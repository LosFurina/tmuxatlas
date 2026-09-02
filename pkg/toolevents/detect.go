package toolevents

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// agentPattern defines how to identify an agent from process cmdline args
type agentPattern struct {
	tool Tool
	// match returns true if the cmdline args indicate this agent
	match func(args []string) bool
}

// agentPatterns lists known agent signatures to look for in the process tree.
// Each pattern inspects the full cmdline (argv) of a process.
var agentPatterns = []agentPattern{
	{
		tool: ToolClaude,
		match: func(args []string) bool {
			return matchBinaryName(args, "claude")
		},
	},
	{
		tool: ToolCopilot,
		match: func(args []string) bool {
			// Copilot CLI runs as node — check if any arg contains "copilot"
			if matchBinaryName(args, "copilot") {
				return true
			}
			return matchNodeScript(args, "copilot")
		},
	},
	{
		tool: ToolCodex,
		match: func(args []string) bool {
			if matchBinaryName(args, "codex") {
				return true
			}
			return matchNodeScript(args, "codex")
		},
	},
	{
		tool: ToolOpenCode,
		match: func(args []string) bool {
			return matchBinaryName(args, "opencode") || matchNodeScript(args, "opencode")
		},
	},
	{tool: ToolAgy, match: func(args []string) bool { return matchBinaryName(args, "agy") }},
}

func detectFromArgs(args []string) (Tool, bool) {
	for _, pat := range agentPatterns {
		if pat.match(args) {
			return pat.tool, true
		}
	}
	return "", false
}

// matchBinaryName checks if the first arg (the binary) has the given base name.
func matchBinaryName(args []string, name string) bool {
	if len(args) == 0 {
		return false
	}
	base := filepath.Base(args[0])
	return base == name
}

// matchNodeScript checks if this is a node process running a script whose
// path contains the given name. Handles patterns like:
//
//	node /usr/lib/node_modules/@openai/codex/bin/codex.js
//	node /home/user/.npm/bin/copilot
func matchNodeScript(args []string, name string) bool {
	if len(args) < 2 {
		return false
	}
	base := filepath.Base(args[0])
	if base != "node" && base != "nodejs" {
		return false
	}
	// Check remaining args for the tool name in the path or filename
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		clean := filepath.Clean(arg)
		for _, component := range strings.Split(clean, string(filepath.Separator)) {
			component = strings.TrimSuffix(strings.ToLower(component), ".js")
			if component == name {
				return true
			}
		}
	}
	return false
}

// DetectAgentInProcessTree walks the process tree rooted at pid and returns
// the first recognized agent tool found. Returns ("", false) if no agent
// is detected. Only inspects direct children of the given PID.
func DetectAgentInProcessTree(pid int) (Tool, bool) {
	children := getChildPIDs(pid)
	for _, cpid := range children {
		args := readCmdline(cpid)
		if len(args) == 0 {
			continue
		}
		if tool, found := detectFromArgs(args); found {
			return tool, true
		}
		// Also check grandchildren (shell → node → copilot)
		grandchildren := getChildPIDs(cpid)
		for _, gpid := range grandchildren {
			gargs := readCmdline(gpid)
			if len(gargs) == 0 {
				continue
			}
			if tool, found := detectFromArgs(gargs); found {
				return tool, true
			}
		}
	}
	return "", false
}

// getChildPIDs returns the PIDs of all direct children of the given PID.
// Uses /proc on Linux.
func getChildPIDs(pid int) []int {
	taskDir := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	data, err := os.ReadFile(taskDir)
	if err != nil {
		// Fallback: scan /proc for processes whose PPid matches
		return getChildPIDsFallback(pid)
	}
	return parsePIDList(string(data))
}

// getChildPIDsFallback scans /proc/*/status for processes with matching PPid.
// Works on Linux when /proc/<pid>/task/<pid>/children is unavailable.
func getChildPIDsFallback(ppid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	ppidStr := strconv.Itoa(ppid)
	var children []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		statusPath := fmt.Sprintf("/proc/%d/status", pid)
		data, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PPid:\t") {
				if strings.TrimPrefix(line, "PPid:\t") == ppidStr {
					children = append(children, pid)
				}
				break
			}
		}
	}
	return children
}

// readCmdline reads /proc/<pid>/cmdline and splits by null bytes.
func readCmdline(pid int) []string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return nil
	}
	// cmdline is null-delimited, trim trailing null
	s := strings.TrimRight(string(data), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// parsePIDList parses a space-separated list of PIDs
func parsePIDList(s string) []int {
	var pids []int
	for _, field := range strings.Fields(s) {
		pid, err := strconv.Atoi(field)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
