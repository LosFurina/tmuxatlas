package fleethealth

import (
	"testing"
	"time"
)

func boolPointer(value bool) *bool { return &value }
func intPointer(value int) *int    { return &value }

func TestEvaluateHealthClassification(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	pass := true
	base := Facts{
		HostID: "host-a", Online: boolPointer(true),
		LastStateSync: now.Add(-time.Second), Version: "v0.7.0", HubVersion: "v0.7.0",
		ProtocolVersion: intPointer(1), ProtocolMin: intPointer(1), ProtocolMax: intPointer(1),
		Checks: []Check{{Name: "agent", OK: &pass}},
	}
	tests := []struct {
		name   string
		mutate func(*Facts)
		want   string
	}{
		{name: "healthy", want: "healthy"},
		{name: "offline", mutate: func(f *Facts) { f.Online = boolPointer(false) }, want: "offline"},
		{name: "stale", mutate: func(f *Facts) { f.LastStateSync = now.Add(-10 * time.Minute) }, want: "stale"},
		{name: "behind", mutate: func(f *Facts) { f.Version = "v0.6.0" }, want: "version-behind"},
		{name: "ahead", mutate: func(f *Facts) { f.Version = "v0.8.0" }, want: "version-ahead"},
		{name: "explicit incompatible", mutate: func(f *Facts) { f.ProtocolVersion = intPointer(2) }, want: "incompatible"},
		{name: "unknown compatibility", mutate: func(f *Facts) { f.ProtocolVersion = nil }, want: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := base
			if test.mutate != nil {
				test.mutate(&facts)
			}
			if got := Evaluate(facts, now, 2*time.Minute).Summary; got != test.want {
				t.Fatalf("summary = %q, want %q", got, test.want)
			}
		})
	}
}

func TestVersionDoesNotImplyIncompatibility(t *testing.T) {
	now := time.Now()
	pass := true
	record := Evaluate(Facts{
		HostID: "host-a", Online: boolPointer(true), LastStateSync: now,
		Version: "v99.0.0", HubVersion: "v1.0.0",
		Checks: []Check{{Name: "agent", OK: &pass}},
	}, now, time.Minute)
	for _, reason := range record.Reasons {
		if reason.Code == "incompatible" {
			t.Fatal("application version ordering must not imply protocol incompatibility")
		}
	}
}

func TestDockerRemediationIsNonExecutingComposeGuidance(t *testing.T) {
	now := time.Now()
	record := Evaluate(Facts{
		HostID: "hub", Deployment: "docker", Online: boolPointer(true),
		LastStateSync: now, Version: "v0.6.0", HubVersion: "v0.7.0",
	}, now, time.Minute)
	found := false
	for _, reason := range record.Reasons {
		if reason.Code == "version-behind" {
			found = true
			if reason.Remediation != "docker compose pull && docker compose up -d" {
				t.Fatalf("remediation = %q", reason.Remediation)
			}
		}
	}
	if !found {
		t.Fatal("missing version-behind reason")
	}
}
