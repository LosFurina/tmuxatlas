package toolevents

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Detector periodically scans tmux panes for known agent processes by
// inspecting the process tree. When an agent is detected in a pane that
// doesn't already have a tracked event, a synthetic "active" event is
// recorded. This provides passive agent detection for tools that may not
// have hooks configured (or whose hooks haven't fired yet).
type Detector struct {
	tracker     *Tracker
	listPane    PaneListFunc
	detectAgent func(int) (Tool, bool)
	interval    time.Duration
	log         *logrus.Entry
	hostID      string // local host fingerprint (for multi-host)
	hostName    string // local host display name

	// seen tracks panes where an agent was previously detected, so we
	// don't re-broadcast every scan cycle. Entries are removed when the
	// agent process is no longer found.
	mu                     sync.Mutex
	scanMu                 sync.Mutex
	seen                   map[PaneKey]Tool
	generation             map[PaneKey]uint64
	beforePassiveDeparture func() // test seam; nil in production
}

var passiveTools = map[Tool]bool{ToolAgy: true}

// NewDetector creates a new agent detector.
func NewDetector(tracker *Tracker, listPane PaneListFunc, interval time.Duration) *Detector {
	return &Detector{
		tracker:     tracker,
		listPane:    listPane,
		detectAgent: DetectAgentInProcessTree,
		interval:    interval,
		log:         logrus.WithField("component", "agent-detector"),
		seen:        make(map[PaneKey]Tool),
		generation:  make(map[PaneKey]uint64),
	}
}

// SetHost sets the local host identity for multi-host event stamping.
func (d *Detector) SetHost(id, name string) {
	d.hostID = id
	d.hostName = name
}

// DetectedPanes returns the set of pane IDs where an agent was detected
// via process tree inspection.
func (d *Detector) DetectedPanes() map[string]Tool {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make(map[string]Tool, len(d.seen))
	for key, tool := range d.seen {
		if key.Pane != "" {
			result[key.Pane] = tool
		}
	}
	return result
}

// PaneInfo returns the session/window context for a detected pane.
func (d *Detector) PaneInfo(paneID string) PaneInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	for key := range d.seen {
		if key.Pane == paneID {
			return PaneInfo{
				PaneID:  key.Pane,
				Session: key.Session,
				Window:  key.Window,
			}
		}
	}
	return PaneInfo{}
}

func (d *Detector) PassiveToken(paneID string, tool Tool) (PaneInfo, uint64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for key, seenTool := range d.seen {
		if key.Pane == paneID && seenTool == tool {
			return PaneInfo{PaneID: key.Pane, Session: key.Session, Window: key.Window}, d.generation[key], true
		}
	}
	return PaneInfo{}, 0, false
}

func (d *Detector) ValidatePassive(paneID string, tool Tool, generation uint64) (PaneInfo, bool) {
	info, actual, ok := d.PassiveToken(paneID, tool)
	return info, ok && actual == generation
}

// RecordPassiveActive serializes validation and projection with scans. The
// tracker broadcast occurs after detector state validation and never under d.mu.
func (d *Detector) RecordPassiveActive(paneID string, tool Tool, generation uint64, hostID, hostName string) bool {
	d.scanMu.Lock()
	info, ok := d.ValidatePassive(paneID, tool, generation)
	if !ok {
		d.scanMu.Unlock()
		return false
	}
	d.tracker.Record(&Event{Tool: tool, Status: StatusActive, Host: hostID, HostName: hostName, Session: info.Session, Window: info.Window, Pane: paneID, Message: "output resumed", AutoDetected: true})
	d.scanMu.Unlock()
	return true
}

// Run starts the detection loop. It blocks until ctx is cancelled.
func (d *Detector) Run(ctx context.Context) {
	d.log.WithField("interval", d.interval).Info("starting agent detector")

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.log.Info("stopping agent detector")
			return
		case <-ticker.C:
			d.detect()
		}
	}
}

// detect scans all panes and looks for agent processes.
func (d *Detector) detect() {
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	panes, err := d.listPane()
	if err != nil {
		d.log.WithError(err).Warn("agent pane inventory failed; preserving detector state")
		return
	}
	if len(panes) == 0 {
		// An empty successful inventory is authoritative and falls through to
		// departure cleanup below.
	}

	// Get currently tracked events (hook-based) to avoid interfering
	tracked := make(map[PaneKey]Tool)
	for _, evt := range d.tracker.GetAll() {
		tracked[PaneKey{
			Host:    evt.Host,
			Session: evt.Session,
			Window:  evt.Window,
			Pane:    evt.Pane,
		}] = evt.Tool
	}
	d.tracker.mu.RLock()
	for key, evt := range d.tracker.lastActive {
		tracked[key] = evt.Tool
	}
	d.tracker.mu.RUnlock()

	// Track which panes still have agents this cycle.
	stillPresent := make(map[PaneKey]bool)
	var departed []struct {
		key  PaneKey
		tool Tool
	}
	var arrivals []struct {
		key  PaneKey
		tool Tool
	}

	for _, pane := range panes {
		if pane.PID == 0 || pane.Session == "" {
			continue
		}

		key := PaneKey{
			Session: pane.Session,
			Window:  pane.Window,
			Pane:    pane.PaneID,
		}

		tool, found := d.detectAgent(pane.PID)
		if !found {
			continue
		}

		d.mu.Lock()
		previous, alreadySeen := d.seen[key]
		transitioned := alreadySeen && previous != tool
		if !alreadySeen || transitioned {
			d.generation[key]++
		}
		d.seen[key] = tool
		d.mu.Unlock()

		// Passive tools remain detector-owned even while their synthetic waiting
		// projection is present. A passive-to-other-tool transition must bypass
		// the stale projection that it is about to clear.
		trackedTool, isTracked := tracked[key]
		if isTracked && !passiveTools[tool] && !transitioned {
			continue
		}

		stillPresent[key] = true

		if transitioned && passiveTools[previous] {
			// Complete the passive projection before starting the replacement
			// lifecycle. Incrementing generation above invalidates any stale
			// silence-monitor resume token for the old tool.
			departed = append(departed, struct {
				key  PaneKey
				tool Tool
			}{key, previous})
		}
		if alreadySeen && !transitioned {
			continue
		}
		// A hook may have already started the replacement lifecycle between
		// scans. In that case clearing Agy is sufficient; do not overwrite the
		// replacement tool's current status with a synthetic active event.
		if !isTracked || trackedTool != tool {
			arrivals = append(arrivals, struct {
				key  PaneKey
				tool Tool
			}{key, tool})
		}
	}

	// Clean up panes where the agent is no longer detected
	d.mu.Lock()
	for key := range d.seen {
		if !stillPresent[key] {
			d.log.WithFields(logrus.Fields{
				"session": key.Session,
				"window":  key.Window,
				"pane":    key.Pane,
			}).Debug("agent no longer detected in pane")
			tool := d.seen[key]
			delete(d.seen, key)
			if passiveTools[tool] {
				departed = append(departed, struct {
					key  PaneKey
					tool Tool
				}{key, tool})
			}
		}
	}
	d.mu.Unlock()
	if len(departed) > 0 && d.beforePassiveDeparture != nil {
		d.beforePassiveDeparture()
	}
	for _, departure := range departed {
		d.tracker.Record(&Event{Tool: departure.tool, Status: StatusCompleted, Host: d.hostID, HostName: d.hostName, Session: departure.key.Session, Window: departure.key.Window, Pane: departure.key.Pane, Message: "auto-cleared: agent no longer detected", AutoDetected: true})
	}
	for _, arrival := range arrivals {
		d.log.WithFields(logrus.Fields{
			"tool":    arrival.tool,
			"session": arrival.key.Session,
			"window":  arrival.key.Window,
			"pane":    arrival.key.Pane,
		}).Debug("detected agent via process tree")
		d.tracker.Record(&Event{Tool: arrival.tool, Status: StatusActive, Host: d.hostID, HostName: d.hostName, Session: arrival.key.Session, Window: arrival.key.Window, Pane: arrival.key.Pane, Message: "auto-detected", AutoDetected: true})
	}
}
