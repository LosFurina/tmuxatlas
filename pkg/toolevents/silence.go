package toolevents

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TmuxClient is the subset of tmux.Client used by SilenceMonitor
type TmuxClient interface {
	CapturePaneContent(paneID string) (string, error)
}

// silenceThreshold is how long a pane must be quiet before we check for prompts
const silenceThreshold = 10 * time.Second

// maxSilenceChecks is the number of times we'll capture-pane after a pane
// goes silent. After this many checks with no prompt detected, we stop
// until output resumes.
const maxSilenceChecks = 2

// monitoredPane tracks state for a single monitored pane
type monitoredPane struct {
	tool          Tool
	session       string // tmux session name (captured at sync time)
	window        int    // tmux window index (captured at sync time)
	generation    uint64
	lastOutput    time.Time // last time we saw output for this pane
	prompted      bool      // true if we already recorded a waiting event
	silenceChecks int       // how many times we've checked since going silent
}

// SilenceMonitor watches panes running non-Claude agents. It tracks output
// activity via RecordOutput (called from the %output handler) and periodically
// checks panes that have gone quiet for input prompts via capture-pane.
type SilenceMonitor struct {
	tracker  *Tracker
	detector *Detector
	client   TmuxClient
	log      *logrus.Entry
	hostID   string // local host fingerprint (for multi-host)
	hostName string // local host display name

	mu        sync.Mutex
	monitored map[string]*monitoredPane // paneID → state
}

// NewSilenceMonitor creates a new SilenceMonitor.
func NewSilenceMonitor(tracker *Tracker, detector *Detector, client TmuxClient) *SilenceMonitor {
	return &SilenceMonitor{
		tracker:   tracker,
		detector:  detector,
		client:    client,
		log:       logrus.WithField("component", "silence-monitor"),
		monitored: make(map[string]*monitoredPane),
	}
}

// SetHost sets the local host identity for multi-host event stamping.
func (sm *SilenceMonitor) SetHost(id, name string) {
	sm.hostID = id
	sm.hostName = name
}

// RecordOutput notes that a pane produced output, resetting its silence timer.
// Called from the control mode %output handler.
func (sm *SilenceMonitor) RecordOutput(paneID string) {
	sm.mu.Lock()
	var resume *monitoredPane
	if mp, ok := sm.monitored[paneID]; ok {
		mp.lastOutput = time.Now()
		if mp.prompted || mp.silenceChecks > 0 {
			sm.log.WithField("pane", paneID).Trace("output resumed, resetting silence state")
			if mp.prompted && passiveTools[mp.tool] {
				copy := *mp
				resume = &copy
			}
			mp.prompted = false
			mp.silenceChecks = 0
		}
	}
	sm.mu.Unlock()
	if resume != nil {
		sm.detector.RecordPassiveActive(paneID, resume.tool, resume.generation, sm.hostID, sm.hostName)
	}
}

// Run periodically syncs the set of monitored panes with the detector's
// detected panes, then checks quiet panes for input prompts.
// Blocks until ctx is cancelled.
func (sm *SilenceMonitor) Run(ctx context.Context) {
	sm.log.Info("starting silence monitor")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sm.log.Info("stopping silence monitor")
			return
		case <-ticker.C:
			sm.sync()
			sm.checkSilentPanes()
		}
	}
}

// sync updates the set of monitored panes based on current detections.
func (sm *SilenceMonitor) sync() {
	detected := sm.detector.DetectedPanes()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.log.WithFields(logrus.Fields{
		"detected_count":  len(detected),
		"monitored_count": len(sm.monitored),
		"detected":        detected,
	}).Trace("syncing monitored panes")

	// Add new non-Claude panes
	for paneID, tool := range detected {
		if tool == ToolClaude {
			continue // Claude has native waiting hooks
		}
		info, generation, ok := sm.detector.PassiveToken(paneID, tool)
		if !ok {
			continue
		}
		if mp, already := sm.monitored[paneID]; already {
			if mp.tool == tool && mp.generation == generation {
				continue
			}
			// The pane changed tools or was redetected. Replace the monitor
			// state so a waiting projection and resume token from the prior
			// generation cannot leak into the new lifecycle.
			delete(sm.monitored, paneID)
		}

		sm.log.WithFields(logrus.Fields{
			"pane":    paneID,
			"tool":    tool,
			"session": info.Session,
			"window":  info.Window,
		}).Debug("now monitoring pane for silence")

		sm.monitored[paneID] = &monitoredPane{
			tool:       tool,
			session:    info.Session,
			window:     info.Window,
			generation: generation,
			lastOutput: time.Now(), // assume active when first detected
		}
	}

	// Remove departed panes
	for paneID := range sm.monitored {
		if _, stillPresent := detected[paneID]; !stillPresent {
			sm.log.WithField("pane", paneID).Debug("stopping silence monitoring for pane")
			delete(sm.monitored, paneID)
		}
	}
}

// checkSilentPanes captures content from panes that have been quiet longer
// than silenceThreshold and checks for input prompts.
func (sm *SilenceMonitor) checkSilentPanes() {
	now := time.Now()

	// Collect panes to check under lock
	sm.mu.Lock()
	if len(sm.monitored) == 0 {
		sm.mu.Unlock()
		return
	}
	type checkTarget struct {
		paneID  string
		tool    Tool
		session string
		window  int
	}
	var targets []checkTarget
	for paneID, mp := range sm.monitored {
		if mp.prompted || mp.silenceChecks >= maxSilenceChecks {
			continue // already handled or exhausted checks
		}
		if now.Sub(mp.lastOutput) >= silenceThreshold {
			mp.silenceChecks++
			targets = append(targets, checkTarget{paneID: paneID, tool: mp.tool, session: mp.session, window: mp.window})
		}
	}
	sm.mu.Unlock()

	for _, t := range targets {
		sm.log.WithFields(logrus.Fields{
			"pane": t.paneID,
			"tool": t.tool,
		}).Trace("pane has been silent, checking for prompt")

		content, err := sm.client.CapturePaneContent(t.paneID)
		if err != nil {
			sm.log.WithError(err).WithField("pane", t.paneID).Warn("failed to capture pane content")
			continue
		}

		sm.log.WithFields(logrus.Fields{
			"pane":    t.paneID,
			"content": fmt.Sprintf("%.200s", content),
		}).Trace("capture-pane content")

		result := DetectPrompt(content)

		if result.IsPrompt {
			sm.log.WithFields(logrus.Fields{
				"pane":    t.paneID,
				"tool":    t.tool,
				"message": result.Message,
			}).Debug("prompt detected")

			sm.tracker.Record(&Event{
				Tool:         t.tool,
				Status:       StatusWaiting,
				Host:         sm.hostID,
				HostName:     sm.hostName,
				Session:      t.session,
				Window:       t.window,
				Pane:         t.paneID,
				Message:      result.Message,
				AutoDetected: true,
			})

			sm.mu.Lock()
			if mp, ok := sm.monitored[t.paneID]; ok {
				mp.prompted = true
			}
			sm.mu.Unlock()
		} else {
			sm.log.WithFields(logrus.Fields{
				"pane": t.paneID,
				"tool": t.tool,
			}).Trace("no prompt detected")
		}
	}
}
