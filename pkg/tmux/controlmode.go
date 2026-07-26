package tmux

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultRefreshDelay is the debounce window for coalescing notifications
	DefaultRefreshDelay = 100 * time.Millisecond

	// controlSessionName is the tmux session used for the control mode client
	controlSessionName = "tmuxatlas-ctrl"

	// maxBackoff caps the reconnect backoff duration
	maxBackoff = 30 * time.Second
)

// ControlSessionName returns the name of the control mode session so it can be
// filtered from session listings.
func ControlSessionName() string {
	return controlSessionName
}

// Notification represents a parsed control mode notification
type Notification struct {
	Type string   // e.g. "sessions-changed", "window-add", "exit"
	Args []string // parsed arguments after the type
	Raw  string   // raw argument string (unparsed, for %output data length)
}

// ControlMode manages a persistent tmux control mode connection that receives
// real-time notifications about state changes. When a state-change notification
// arrives, it triggers a debounced full refresh via the existing Client methods.
type ControlMode struct {
	client   *Client
	onChange func([]*Session)
	onOutput func(paneID string, dataLen int) // called on %output notifications
	log      *logrus.Entry

	onConnect    func()
	onDisconnect func()

	refreshDelay time.Duration

	mu           sync.Mutex
	refreshTimer *time.Timer
}

// ControlModeOption configures a ControlMode instance
type ControlModeOption func(*ControlMode)

// WithRefreshDelay sets the debounce delay for coalescing notifications
func WithRefreshDelay(d time.Duration) ControlModeOption {
	return func(cm *ControlMode) {
		cm.refreshDelay = d
	}
}

// WithOnConnect sets a callback invoked when control mode connects
func WithOnConnect(fn func()) ControlModeOption {
	return func(cm *ControlMode) {
		cm.onConnect = fn
	}
}

// WithOnDisconnect sets a callback invoked when control mode disconnects
func WithOnDisconnect(fn func()) ControlModeOption {
	return func(cm *ControlMode) {
		cm.onDisconnect = fn
	}
}

// WithOnOutput sets a callback invoked on %output notifications with the pane ID and data length
func WithOnOutput(fn func(paneID string, dataLen int)) ControlModeOption {
	return func(cm *ControlMode) {
		cm.onOutput = fn
	}
}

// NewControlMode creates a new control mode client
func NewControlMode(client *Client, onChange func([]*Session), opts ...ControlModeOption) *ControlMode {
	cm := &ControlMode{
		client:       client,
		onChange:     onChange,
		refreshDelay: DefaultRefreshDelay,
		log:          logrus.WithField("component", "controlmode"),
	}
	for _, opt := range opts {
		opt(cm)
	}
	return cm
}

// Run starts the control mode connection. It blocks until ctx is cancelled,
// automatically reconnecting on disconnection with exponential backoff.
func (cm *ControlMode) Run(ctx context.Context) {
	cm.log.Info("starting control mode client")

	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		err := cm.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		// Disconnected — notify and start backoff
		if cm.onDisconnect != nil {
			cm.onDisconnect()
		}

		cm.log.WithError(err).WithField("backoff", backoff).Warn("control mode disconnected, reconnecting")

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff capped at maxBackoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce spawns a control mode process and reads notifications until it
// disconnects or the context is cancelled.
func (cm *ControlMode) runOnce(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, cm.client.tmuxPath, "-C", "new-session", "-A", "-s", controlSessionName)

	// Provide a pipe for stdin so tmux control mode doesn't see EOF on /dev/null
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	cm.log.Info("control mode connected")

	// Reset backoff on successful connection
	if cm.onConnect != nil {
		cm.onConnect()
	}

	// Read notifications
	readErr := cm.readLoop(ctx, stdout)

	// Clean up process
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	// Cancel any pending refresh
	cm.mu.Lock()
	if cm.refreshTimer != nil {
		cm.refreshTimer.Stop()
		cm.refreshTimer = nil
	}
	cm.mu.Unlock()

	return readErr
}

// readLoop reads lines from the control mode stdout and dispatches notifications.
func (cm *ControlMode) readLoop(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)

	// Increase scanner buffer for potentially large %output lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()

		// Only process notification lines (starting with %)
		if !strings.HasPrefix(line, "%") {
			continue
		}

		n := parseNotification(line)
		if n == nil {
			continue
		}

		cm.handleNotification(n)
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

// parseNotification parses a control mode notification line.
// Returns nil for lines that aren't valid notifications.
func parseNotification(line string) *Notification {
	// Strip the leading %
	line = line[1:]

	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}

	n := &Notification{
		Type: parts[0],
	}
	if len(parts) > 1 {
		n.Raw = parts[1]
		n.Args = strings.Fields(parts[1])
	}
	return n
}

// handleNotification processes a parsed notification.
func (cm *ControlMode) handleNotification(n *Notification) {
	switch n.Type {
	// State-change notifications — trigger debounced refresh
	case "sessions-changed",
		"session-changed",
		"session-renamed",
		"session-window-changed",
		"window-add",
		"window-close",
		"window-renamed",
		"unlinked-window-add",
		"unlinked-window-close",
		"unlinked-window-renamed",
		"layout-change",
		"pane-mode-changed",
		"client-session-changed",
		"client-detached":
		cm.log.WithField("notification", n.Type).Debug("state change notification")
		cm.scheduleRefresh()

	case "exit":
		cm.log.WithField("args", n.Args).Warn("received exit notification")
		// readLoop will return on EOF

	case "output":
		// %output %<pane-id> <data>
		// Forward pane activity to the onOutput callback
		if cm.onOutput != nil && len(n.Args) >= 1 {
			paneID := n.Args[0]
			// Data length: raw string minus pane ID prefix and space
			dataLen := len(n.Raw)
			if idx := strings.IndexByte(n.Raw, ' '); idx >= 0 {
				dataLen = len(n.Raw) - idx - 1
			}
			if dataLen > 0 {
				cm.onOutput(paneID, dataLen)
			}
		}

	// Ignore other non-state notifications
	case "extended-output", "begin", "end", "error",
		"continue", "pause", "message",
		"paste-buffer-changed", "paste-buffer-deleted",
		"subscription-changed", "config-error":
		// no-op

	default:
		cm.log.WithField("notification", n.Type).Debug("unhandled notification")
	}
}

// scheduleRefresh debounces state refresh calls. Multiple notifications within
// the refresh delay window are coalesced into a single refresh.
func (cm *ControlMode) scheduleRefresh() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.refreshTimer != nil {
		cm.refreshTimer.Stop()
	}

	cm.refreshTimer = time.AfterFunc(cm.refreshDelay, cm.doRefresh)
}

// doRefresh performs a full state refresh and calls the onChange callback.
func (cm *ControlMode) doRefresh() {
	sessions, err := cm.client.ListSessions()
	if err != nil {
		cm.log.WithError(err).Warn("failed to refresh sessions")
		return
	}

	// Filter out the control mode session
	filtered := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		if s.Name != controlSessionName {
			filtered = append(filtered, s)
		}
	}

	if cm.onChange != nil {
		cm.onChange(filtered)
	}
}
