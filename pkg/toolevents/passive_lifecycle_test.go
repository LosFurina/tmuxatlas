package toolevents

import (
	"testing"
	"time"
)

func passiveFixture() (*Tracker, *Detector, *SilenceMonitor, PaneKey) {
	tracker := NewTracker()
	detector := NewDetector(tracker, func() ([]PaneInfo, error) { return nil, nil }, time.Second)
	key := PaneKey{Session: "AGY", Window: 0, Pane: "%1"}
	detector.seen[key] = ToolAgy
	detector.generation[key] = 1
	monitor := NewSilenceMonitor(tracker, detector, nil)
	monitor.monitored[key.Pane] = &monitoredPane{tool: ToolAgy, session: key.Session, window: key.Window, generation: 1, prompted: true}
	return tracker, detector, monitor, key
}

func TestPassiveWaitResumeEmitsActiveExactlyOnce(t *testing.T) {
	tracker, _, monitor, _ := passiveFixture()
	sub := tracker.Subscribe()
	monitor.RecordOutput("%1")
	monitor.RecordOutput("%1")
	select {
	case event := <-sub:
		if event.Status != StatusActive || !event.AutoDetected {
			t.Fatalf("event=%#v", event)
		}
	default:
		t.Fatal("missing active")
	}
	select {
	case event := <-sub:
		t.Fatalf("duplicate=%#v", event)
	default:
	}
}

func TestPassiveDepartureAndRedetectRejectOldResumeToken(t *testing.T) {
	tracker, detector, monitor, key := passiveFixture()
	sub := tracker.Subscribe()
	delete(detector.seen, key)
	monitor.RecordOutput(key.Pane)
	select {
	case event := <-sub:
		t.Fatalf("late departure callback=%#v", event)
	default:
	}
	detector.seen[key] = ToolAgy
	detector.generation[key] = 2
	monitor.monitored[key.Pane].prompted = true
	monitor.monitored[key.Pane].generation = 1
	monitor.RecordOutput(key.Pane)
	select {
	case event := <-sub:
		t.Fatalf("old generation callback=%#v", event)
	default:
	}
}

func TestReconcilerSkipsPassiveAutoDetectedEvent(t *testing.T) {
	tracker := NewTracker()
	tracker.Record(&Event{Tool: ToolAgy, Status: StatusWaiting, Session: "AGY", Pane: "%1", AutoDetected: true})
	reconciler := NewReconciler(tracker, func(string) PaneState { t.Fatal("passive event must not reach reconciler lookup"); return PaneState{} }, time.Second)
	reconciler.reconcile()
	if len(tracker.GetAll()) != 1 {
		t.Fatal("passive state was cleared")
	}
}

func TestPassiveResumeBarrierCannotRecordAfterDeparture(t *testing.T) {
	tracker, detector, _, key := passiveFixture()
	sub := tracker.Subscribe()
	detector.scanMu.Lock()
	done := make(chan bool, 1)
	go func() { done <- detector.RecordPassiveActive(key.Pane, ToolAgy, 1, "", "") }()
	time.Sleep(10 * time.Millisecond)
	detector.mu.Lock()
	delete(detector.seen, key)
	detector.mu.Unlock()
	detector.scanMu.Unlock()
	if <-done {
		t.Fatal("resume recorded after detector departure")
	}
	select {
	case event := <-sub:
		t.Fatalf("unexpected event=%#v", event)
	default:
	}
}

func TestDetectDepartureOrdersCompletedBeforeBlockedResume(t *testing.T) {
	tracker := NewTracker()
	sub := tracker.Subscribe()
	detector := NewDetector(tracker, func() ([]PaneInfo, error) { return nil, nil }, time.Second)
	key := PaneKey{Session: "AGY", Pane: "%1"}
	detector.seen[key] = ToolAgy
	detector.generation[key] = 1
	ready, release := make(chan struct{}), make(chan struct{})
	detector.beforePassiveDeparture = func() { close(ready); <-release }
	done := make(chan bool, 1)
	go func() { detector.detect(); done <- true }()
	<-ready
	resume := make(chan bool, 1)
	go func() { resume <- detector.RecordPassiveActive(key.Pane, ToolAgy, 1, "", "") }()
	close(release)
	<-done
	event := <-sub
	if event.Status != StatusCompleted {
		t.Fatalf("first event=%#v", event)
	}
	if <-resume {
		t.Fatal("resume active emitted after real detect departure")
	}
	select {
	case event := <-sub:
		t.Fatalf("unexpected=%#v", event)
	default:
	}
}
