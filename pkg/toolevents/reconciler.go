package toolevents

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// PaneState describes the current state of a tmux pane for reconciliation
type PaneState struct {
	Exists         bool
	CurrentCommand string
	PID            int
}

// PaneLookupFunc returns the state of a pane given its ID.
// If the pane does not exist, it should return PaneState{Exists: false}.
type PaneLookupFunc func(paneID string) PaneState

// PaneInfo contains identifying information for a tmux pane, used by the
// agent detector to scan panes for running agents.
type PaneInfo struct {
	PaneID  string
	Session string // session name
	Window  int    // window index
	PID     int    // pane process PID
}

// PaneListFunc returns a pane inventory. An error is not equivalent to an
// authoritative empty inventory: Detector preserves its ownership state.
type PaneListFunc func() ([]PaneInfo, error)

// shells is the set of commands that indicate no agent is running
var shells = map[string]bool{
	"zsh":  true,
	"bash": true,
	"fish": true,
	"sh":   true,
	"dash": true,
	"tcsh": true,
	"csh":  true,
	"ksh":  true,
	"pwsh": true,
}

// Reconciler periodically checks tracked tool events against actual tmux pane
// state, clearing events whose agent process is no longer running.
type Reconciler struct {
	tracker  *Tracker
	lookup   PaneLookupFunc
	interval time.Duration
	log      *logrus.Entry
}

// NewReconciler creates a reconciler that checks tracked events against pane state.
func NewReconciler(tracker *Tracker, lookup PaneLookupFunc, interval time.Duration) *Reconciler {
	return &Reconciler{
		tracker:  tracker,
		lookup:   lookup,
		interval: interval,
		log:      logrus.WithField("component", "tool-reconciler"),
	}
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	r.log.WithField("interval", r.interval).Info("starting tool event reconciler")

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("stopping tool event reconciler")
			return
		case <-ticker.C:
			r.reconcile()
		}
	}
}

// reconcile checks all tracked events and clears any whose pane no longer
// has an agent running.
func (r *Reconciler) reconcile() {
	events := r.tracker.GetAll()
	if len(events) == 0 {
		return
	}

	for _, evt := range events {
		// Passive process detections are owned by Detector, which distinguishes
		// an inventory error from an authoritative departure and emits exactly
		// one completion. Reconciliation remains the hook-event safety net.
		if evt.AutoDetected && passiveTools[evt.Tool] {
			continue
		}
		if evt.Pane == "" {
			continue
		}

		ps := r.lookup(evt.Pane)

		shouldClear := false
		if !ps.Exists {
			r.log.WithField("pane", evt.Pane).Debug("pane no longer exists, clearing event")
			shouldClear = true
		} else if shells[ps.CurrentCommand] {
			r.log.WithFields(logrus.Fields{
				"pane":    evt.Pane,
				"command": ps.CurrentCommand,
			}).Debug("pane foreground is shell, clearing event")
			shouldClear = true
		}

		if shouldClear {
			// Record a synthetic completed event to clear tracking and notify subscribers
			r.tracker.Record(&Event{
				Tool:    evt.Tool,
				Status:  StatusCompleted,
				Session: evt.Session,
				Window:  evt.Window,
				Pane:    evt.Pane,
				Message: "auto-cleared: agent no longer running",
			})
		}
	}
}
