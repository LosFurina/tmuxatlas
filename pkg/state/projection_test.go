package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStableKeysKeepSameNamedSessionsSeparate(t *testing.T) {
	first := NewSessionKey("host-a", "work")
	second := NewSessionKey("host-b", "work")
	if first == second {
		t.Fatalf("same-named sessions on separate hosts collided: %q", first)
	}
	if got, want := NewPaneKey("host/a", "work ops", "@1", "%2"), PaneKey("host/host%2Fa/session/work%20ops/window/@1/pane/%252"); got != want {
		t.Fatalf("pane key = %q, want %q", got, want)
	}
}

func TestStateEnvelopeJSONRoundTrip(t *testing.T) {
	session := Session{
		Key: NewSessionKey("host-a", "work"), HostKey: NewHostKey("host-a"),
		HostID: "host-a", Name: "work",
	}
	tests := []struct {
		name     string
		value    any
		contains []string
	}{
		{
			name: "snapshot",
			value: SnapshotEnvelope{
				Type: EnvelopeSnapshot, SchemaVersion: SchemaVersion,
				InstanceID: "instance-a", Revision: 7,
				State: Projection{
					Hosts: map[HostKey]Host{
						NewHostKey("host-a"): {
							Key: NewHostKey("host-a"), ID: "host-a", DisplayName: "same",
						},
						NewHostKey("host-b"): {
							Key: NewHostKey("host-b"), ID: "host-b", DisplayName: "same",
						},
					},
					Sessions: map[SessionKey]Session{session.Key: session},
					Windows:  map[WindowKey]Window{}, Panes: map[PaneKey]Pane{},
					ToolEvents: map[ToolEventKey]ToolEvent{},
					Activity:   map[ActivityKey]Activity{}, Health: map[HostKey]Health{},
				},
			},
			contains: []string{`"type":"snapshot"`, `"schema_version":1`, `"instance_id":"instance-a"`, `"revision":7`},
		},
		{
			name: "delta",
			value: DeltaEnvelope{
				Type: EnvelopeDelta, SchemaVersion: SchemaVersion,
				InstanceID: "instance-a", BaseRevision: 7, Revision: 8,
				Operations: []Operation{{Kind: OperationUpsertSession, Session: &session}},
			},
			contains: []string{`"type":"delta"`, `"base_revision":7`, `"kind":"upsert-session"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			for _, fragment := range tt.contains {
				if !strings.Contains(string(data), fragment) {
					t.Errorf("JSON %s does not contain %s", data, fragment)
				}
			}
		})
	}
}

func TestOperationValidation(t *testing.T) {
	host := Host{Key: NewHostKey("host-a"), ID: "host-a"}
	tests := []struct {
		name    string
		op      Operation
		wantErr bool
	}{
		{name: "typed upsert", op: Operation{Kind: OperationUpsertHost, Host: &host}},
		{name: "missing typed value", op: Operation{Kind: OperationUpsertHost}, wantErr: true},
		{name: "remove key", op: Operation{Kind: OperationRemoveHost, Key: string(host.Key)}},
		{name: "missing remove key", op: Operation{Kind: OperationRemoveHost}, wantErr: true},
		{name: "unknown", op: Operation{Kind: "explode"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.op.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnvelopeValidation(t *testing.T) {
	valid := DeltaEnvelope{
		Type: EnvelopeDelta, SchemaVersion: SchemaVersion, InstanceID: "instance",
		BaseRevision: 3, Revision: 4,
		Operations: []Operation{{Kind: OperationRemoveHost, Key: "host/a"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.SchemaVersion++
	if err := valid.Validate(); err == nil {
		t.Fatal("expected incompatible schema to fail validation")
	}
}
