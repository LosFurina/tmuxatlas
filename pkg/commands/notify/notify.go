package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/socket"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

// stdinEvent represents the JSON payload that agent hooks pass via stdin.
// Supports Codex hook events (SessionStart, Stop) and can be extended for others.
type stdinEvent struct {
	HookEventName string `json:"hook_event_name"`
	// Codex Stop hook fields
	LastAssistantMessage *string `json:"last_assistant_message,omitempty"`
}

// parseStdinEvent reads JSON from stdin and maps it to tool/status/message.
// Returns the tool name, status, and message to use for the notification.
func parseStdinEvent(tool string) (string, string, string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(data) == 0 {
		return "", "", "", fmt.Errorf("no input on stdin")
	}

	var evt stdinEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return "", "", "", fmt.Errorf("failed to parse stdin JSON: %w", err)
	}

	status := "active"
	message := "Working"

	switch evt.HookEventName {
	case "SessionStart":
		status = "active"
		message = "Session started"
	case "Stop":
		status = "completed"
		message = "Task complete"
		if evt.LastAssistantMessage != nil && *evt.LastAssistantMessage != "" {
			// Truncate to keep the notification concise
			msg := *evt.LastAssistantMessage
			if len(msg) > 100 {
				msg = msg[:100] + "..."
			}
			message = msg
		}
	default:
		if evt.HookEventName != "" {
			message = evt.HookEventName
		}
	}

	return tool, status, message, nil
}

// codexEvent represents the JSON payload Codex passes as argv[1] to notify
// commands. See: https://developers.openai.com/codex/config-advanced/#notifications
type codexEvent struct {
	Type                 string `json:"type"`
	ThreadID             string `json:"thread-id"`
	TurnID               string `json:"turn-id"`
	CWD                  string `json:"cwd"`
	LastAssistantMessage string `json:"last-assistant-message"`
}

// parseEventData parses a JSON string passed via --event-data (argv) and maps
// it to status/message based on the tool type.
func parseEventData(tool, data string) (string, string, error) {
	switch tool {
	case "codex":
		return parseCodexEventData(data)
	default:
		// Generic: try to extract a status and message from the JSON
		var generic map[string]interface{}
		if err := json.Unmarshal([]byte(data), &generic); err != nil {
			return "", "", fmt.Errorf("failed to parse event JSON: %w", err)
		}
		return "active", "Event received", nil
	}
}

// parseCodexEventData parses Codex's notification JSON.
// Currently only "agent-turn-complete" is emitted.
func parseCodexEventData(data string) (string, string, error) {
	var evt codexEvent
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return "", "", fmt.Errorf("failed to parse codex event JSON: %w", err)
	}

	switch evt.Type {
	case "agent-turn-complete":
		message := "Task complete"
		if evt.LastAssistantMessage != "" {
			msg := evt.LastAssistantMessage
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			message = msg
		}
		return "completed", message, nil
	default:
		// Unknown event type — treat as active
		message := evt.Type
		if message == "" {
			message = "Event received"
		}
		return "active", message, nil
	}
}

// detectTmuxContext auto-detects the current tmux session, window, and pane
// from the environment. Returns session name, window index, pane ID.
func detectTmuxContext() (string, int, string) {
	paneID := os.Getenv("TMUX_PANE")

	// If we have TMUX_PANE, query that specific pane's session and window
	// instead of using display-message which returns the *active* pane's info.
	if paneID != "" {
		session, window := queryPaneContext(paneID)
		if session != "" {
			return session, window, paneID
		}
	}

	// Fallback: use display-message (returns active pane context)
	session, _ := tmuxQuery("#{session_name}")
	winStr, _ := tmuxQuery("#{window_index}")
	winIdx, _ := strconv.Atoi(winStr)

	return strings.TrimSpace(session), winIdx, strings.TrimSpace(paneID)
}

// queryPaneContext gets the session name and window index for a specific pane ID
func queryPaneContext(paneID string) (string, int) {
	cmd := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{session_name}\t#{window_index}")
	out, err := cmd.Output()
	if err != nil {
		return "", 0
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
	if len(parts) != 2 {
		return "", 0
	}
	winIdx, _ := strconv.Atoi(parts[1])
	return parts[0], winIdx
}

func postViaSocket(socketPath string, body []byte) (*http.Response, error) {
	client := &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
	return client.Post("http://localhost/api/tool-event", "application/json", bytes.NewReader(body))
}

func tmuxQuery(format string) (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", format)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func Execute(ctx context.Context, c *cli.Command) error {
	tool := c.String("tool")
	status := c.String("status")
	message := c.String("message")
	session := c.String("session")
	window := int(c.Int("window"))
	pane := c.String("pane")
	log := logrus.WithField("component", "notify")

	log.WithFields(logrus.Fields{
		"tool": tool, "status": status, "message": message,
		"event-data-set": c.IsSet("event-data"), "stdin-set": c.Bool("stdin"),
	}).Trace("notify command invoked")

	// If --event-data is set, parse the JSON blob (passed as argv by agents like Codex)
	if c.IsSet("event-data") {
		rawData := c.String("event-data")
		log.WithField("raw_event_data", rawData).Trace("parsing --event-data")

		evtStatus, evtMessage, err := parseEventData(tool, rawData)
		if err != nil {
			return fmt.Errorf("event-data parse: %w", err)
		}
		log.WithFields(logrus.Fields{
			"parsed_status": evtStatus, "parsed_message": evtMessage,
		}).Trace("event-data parsed")

		if !c.IsSet("status") {
			status = evtStatus
		}
		if !c.IsSet("message") {
			message = evtMessage
		}
	}

	// If --stdin is set, read event JSON from stdin and derive status/message
	if c.Bool("stdin") {
		log.Trace("reading event from stdin")
		stdinTool, stdinStatus, stdinMessage, err := parseStdinEvent(tool)
		if err != nil {
			return fmt.Errorf("stdin parse: %w", err)
		}
		log.WithFields(logrus.Fields{
			"parsed_tool": stdinTool, "parsed_status": stdinStatus, "parsed_message": stdinMessage,
		}).Trace("stdin event parsed")

		tool = stdinTool
		if !c.IsSet("status") {
			status = stdinStatus
		}
		if !c.IsSet("message") {
			message = stdinMessage
		}
	}

	if status == "" {
		return fmt.Errorf("--status is required (or use --event-data/--stdin to read from agent hook input)")
	}

	// Auto-detect tmux context if not provided
	if session == "" || pane == "" {
		log.WithField("TMUX_PANE", os.Getenv("TMUX_PANE")).Trace("auto-detecting tmux context")
		autoSession, autoWindow, autoPane := detectTmuxContext()
		log.WithFields(logrus.Fields{
			"auto_session": autoSession, "auto_window": autoWindow, "auto_pane": autoPane,
		}).Trace("tmux context detected")

		if session == "" {
			session = autoSession
		}
		if !c.IsSet("window") {
			window = autoWindow
		}
		if pane == "" {
			pane = autoPane
		}
	}

	if session == "" {
		return fmt.Errorf("could not detect tmux session; pass --session explicitly")
	}

	evt := &toolevents.Event{
		Tool:    toolevents.Tool(tool),
		Status:  toolevents.Status(status),
		Session: session,
		Window:  window,
		Pane:    pane,
		Message: message,
	}

	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		"tool": tool, "status": status, "session": session,
		"window": window, "pane": pane, "message": message,
	}).Trace("sending event")

	socketPath := c.String("socket")
	if socketPath == "" {
		socketPath = socket.DefaultPath()
	}

	log.WithField("socket", socketPath).Trace("sending via unix socket")
	resp, err := postViaSocket(socketPath, body)
	if err != nil {
		return fmt.Errorf("failed to notify TmuxAtlas through Unix socket %s: %w (make sure the hub or agent service is running)", socketPath, err)
	}
	log.WithField("status_code", resp.StatusCode).Trace("unix socket response")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("TmuxAtlas returned status %d", resp.StatusCode)
	}

	log.WithFields(logrus.Fields{
		"tool": tool, "status": status, "session": session,
		"window": window, "pane": pane,
	}).Debug("notification sent")

	return nil
}

func init() {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:     "tool",
			Aliases:  []string{"t"},
			Usage:    "tool name: claude, codex, copilot, opencode, agy",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "status",
			Aliases: []string{"s"},
			Usage:   "status: active, waiting, completed, error",
		},
		&cli.StringFlag{
			Name:    "message",
			Aliases: []string{"m"},
			Usage:   "human-readable message",
		},
		&cli.StringFlag{
			Name:  "event-data",
			Usage: "agent event JSON passed as argument (used by Codex notify hook)",
		},
		&cli.BoolFlag{
			Name:  "stdin",
			Usage: "read hook event JSON from stdin (for agent hooks that pass context via stdin)",
		},
		&cli.StringFlag{
			Name:  "session",
			Usage: "tmux session name (auto-detected if omitted)",
		},
		&cli.IntFlag{
			Name:  "window",
			Usage: "tmux window index (auto-detected if omitted)",
		},
		&cli.StringFlag{
			Name:  "pane",
			Usage: "tmux pane ID (auto-detected if omitted)",
		},
		&cli.StringFlag{
			Name:    "socket",
			Usage:   "path to TmuxAtlas Unix socket (auto-detected if omitted)",
			Sources: cli.EnvVars("TMUXATLAS_SOCKET", "GUPPI_SOCKET"),
		},
	}

	cmd := &cli.Command{
		Name:  "notify",
		Usage: "send an agent hook event to TmuxAtlas",
		Description: `Notify tmuxatlas of AI tool activity. Used by agent hooks.

Examples:
  tmuxatlas notify -t claude -s waiting -m "Needs approval"
  tmuxatlas notify -t codex -s active
  tmuxatlas notify -t claude -s completed

The tmux session, window, and pane are auto-detected when run inside tmux.`,
		Flags:  flags,
		Action: Execute,
	}

	common.RegisterCommand(cmd)
}
