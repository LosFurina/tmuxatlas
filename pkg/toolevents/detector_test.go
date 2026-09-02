package toolevents

import (
	"errors"
	"testing"
	"time"
)

func TestDetectorPreservesStateOnInventoryErrorAndCompletesPassiveDeparture(t *testing.T) {
	tracker := NewTracker()
	sub := tracker.Subscribe()
	key := PaneKey{Session: "AGY", Window: 0, Pane: "%1"}
	detector := NewDetector(tracker, func() ([]PaneInfo, error) { return nil, errors.New("tmux unavailable") }, time.Second)
	detector.seen[key] = ToolAgy
	detector.detect()
	if got := detector.DetectedPanes()["%1"]; got != ToolAgy {
		t.Fatalf("state lost after inventory error: %q", got)
	}

	detector.listPane = func() ([]PaneInfo, error) { return []PaneInfo{}, nil }
	detector.detect()
	select {
	case event := <-sub:
		if event.Tool != ToolAgy || event.Status != StatusCompleted || !event.AutoDetected {
			t.Fatalf("unexpected departure event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected completed event")
	}
	if _, ok := detector.DetectedPanes()["%1"]; ok {
		t.Fatal("departure must clear detector ownership")
	}
	if len(tracker.GetAll()) != 0 {
		t.Fatal("completed departure must clear state projection")
	}
}

func TestDetectorCompletesPassiveAgyBeforeStartingReplacementTool(t *testing.T) {
	tracker := NewTracker()
	sub := tracker.Subscribe()
	panes := []PaneInfo{{PaneID: "%1", Session: "AGY", Window: 0, PID: 42}}
	tool := ToolAgy
	detector := NewDetector(tracker, func() ([]PaneInfo, error) { return panes, nil }, time.Second)
	detector.detectAgent = func(int) (Tool, bool) { return tool, true }
	detector.detect()
	<-sub // initial passive Agy active projection

	tracker.Record(&Event{Tool: ToolAgy, Status: StatusWaiting, Session: "AGY", Pane: "%1", AutoDetected: true})
	<-sub // synthetic waiting projection
	monitor := NewSilenceMonitor(tracker, detector, nil)
	monitor.sync()
	oldGeneration := monitor.monitored["%1"].generation

	tool = ToolOpenCode
	detector.detect()

	completed := <-sub
	if completed.Tool != ToolAgy || completed.Status != StatusCompleted {
		t.Fatalf("first transition event = %#v, want Agy completed", completed)
	}
	active := <-sub
	if active.Tool != ToolOpenCode || active.Status != StatusActive {
		t.Fatalf("second transition event = %#v, want OpenCode active", active)
	}
	if events := tracker.GetAll(); len(events) != 0 {
		t.Fatalf("stale passive projection survived transition: %#v", events)
	}
	if info, generation, ok := detector.PassiveToken("%1", ToolAgy); ok || info != (PaneInfo{}) || generation == oldGeneration {
		t.Fatalf("old Agy token remains valid: info=%#v generation=%d ok=%v", info, generation, ok)
	}

	monitor.sync()
	monitored := monitor.monitored["%1"]
	if monitored == nil || monitored.tool != ToolOpenCode || monitored.generation == oldGeneration || monitored.prompted {
		t.Fatalf("monitor was not replaced for the new lifecycle: %#v", monitored)
	}
	monitor.RecordOutput("%1")
	select {
	case event := <-sub:
		t.Fatalf("stale Agy resume leaked after transition: %#v", event)
	default:
	}
}

func TestDetectorPreservesReplacementHookWaitingWhenAgyDeparts(t *testing.T) {
	tracker := NewTracker()
	sub := tracker.Subscribe()
	panes := []PaneInfo{{PaneID: "%1", Session: "AGY", Window: 0, PID: 42}}
	tool := ToolAgy
	detector := NewDetector(tracker, func() ([]PaneInfo, error) { return panes, nil }, time.Second)
	detector.detectAgent = func(int) (Tool, bool) { return tool, true }
	detector.detect()
	<-sub // initial passive Agy active projection

	tool = ToolOpenCode
	tracker.Record(&Event{Tool: ToolOpenCode, Status: StatusWaiting, Session: "AGY", Pane: "%1"})
	<-sub // replacement hook waiting before detector scan
	detector.detect()

	completed := <-sub
	if completed.Tool != ToolAgy || completed.Status != StatusCompleted {
		t.Fatalf("transition event = %#v, want Agy completed", completed)
	}
	select {
	case unexpected := <-sub:
		t.Fatalf("replacement hook lifecycle was overwritten: %#v", unexpected)
	default:
	}
	events := tracker.GetAll()
	if len(events) != 1 || events[0].Tool != ToolOpenCode || events[0].Status != StatusWaiting {
		t.Fatalf("replacement waiting state was not preserved: %#v", events)
	}
}
