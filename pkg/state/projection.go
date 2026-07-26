package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// SchemaVersion is the browser-visible canonical state protocol version.
	SchemaVersion = 1

	EnvelopeSnapshot       = "snapshot"
	EnvelopeDelta          = "delta"
	EnvelopeResyncRequired = "resync-required"
	EnvelopeReloadRequired = "reload-required"
)

type (
	HostKey      string
	SessionKey   string
	WindowKey    string
	PaneKey      string
	ToolEventKey string
	ActivityKey  string
)

func keyPart(value string) string {
	return url.PathEscape(value)
}

func NewHostKey(hostID string) HostKey {
	return HostKey("host/" + keyPart(hostID))
}

func NewSessionKey(hostID, sessionName string) SessionKey {
	return SessionKey(fmt.Sprintf("%s/session/%s", NewHostKey(hostID), keyPart(sessionName)))
}

func NewWindowKey(hostID, sessionName, windowID string) WindowKey {
	return WindowKey(fmt.Sprintf("%s/window/%s", NewSessionKey(hostID, sessionName), keyPart(windowID)))
}

func NewPaneKey(hostID, sessionName, windowID, paneID string) PaneKey {
	return PaneKey(fmt.Sprintf("%s/pane/%s", NewWindowKey(hostID, sessionName, windowID), keyPart(paneID)))
}

type Host struct {
	Key         HostKey   `json:"key"`
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Online      bool      `json:"online"`
	Local       bool      `json:"local,omitempty"`
	Version     string    `json:"version,omitempty"`
	LastSeen    time.Time `json:"last_seen,omitempty"`
}

type Session struct {
	Key          SessionKey `json:"key"`
	HostKey      HostKey    `json:"host_key"`
	HostID       string     `json:"host_id"`
	Name         string     `json:"name"`
	TmuxID       string     `json:"tmux_id,omitempty"`
	Attached     bool       `json:"attached"`
	Created      time.Time  `json:"created,omitempty"`
	LastActivity time.Time  `json:"last_activity,omitempty"`
}

type Window struct {
	Key        WindowKey  `json:"key"`
	SessionKey SessionKey `json:"session_key"`
	TmuxID     string     `json:"tmux_id"`
	Name       string     `json:"name"`
	Index      int        `json:"index"`
	Active     bool       `json:"active"`
	Layout     string     `json:"layout,omitempty"`
}

type Pane struct {
	Key            PaneKey   `json:"key"`
	WindowKey      WindowKey `json:"window_key"`
	TmuxID         string    `json:"tmux_id"`
	Index          int       `json:"index"`
	Active         bool      `json:"active"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	CurrentCommand string    `json:"current_command,omitempty"`
	PID            int       `json:"pid,omitempty"`
}

type ToolEvent struct {
	Key          ToolEventKey `json:"key"`
	HostID       string       `json:"host_id"`
	Session      string       `json:"session"`
	Window       string       `json:"window,omitempty"`
	Pane         string       `json:"pane,omitempty"`
	Tool         string       `json:"tool"`
	Status       string       `json:"status"`
	Message      string       `json:"message,omitempty"`
	Timestamp    time.Time    `json:"timestamp"`
	AutoDetected bool         `json:"auto_detected,omitempty"`
}

type Activity struct {
	Key       ActivityKey `json:"key"`
	HostID    string      `json:"host_id"`
	Session   string      `json:"session"`
	Window    string      `json:"window,omitempty"`
	Pane      string      `json:"pane,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Data      any         `json:"data,omitempty"`
}

type Health struct {
	HostKey       HostKey        `json:"host_key"`
	LastStateSync time.Time      `json:"last_state_sync,omitempty"`
	Facts         map[string]any `json:"facts,omitempty"`
}

// Projection is the normalized, browser-visible state owned by the Hub.
// Map keys, rather than display names, are the authoritative identities.
type Projection struct {
	Hosts      map[HostKey]Host           `json:"hosts"`
	Sessions   map[SessionKey]Session     `json:"sessions"`
	Windows    map[WindowKey]Window       `json:"windows"`
	Panes      map[PaneKey]Pane           `json:"panes"`
	ToolEvents map[ToolEventKey]ToolEvent `json:"tool_events"`
	Activity   map[ActivityKey]Activity   `json:"activity"`
	Health     map[HostKey]Health         `json:"health"`
	Metadata   map[string]json.RawMessage `json:"metadata,omitempty"`
}

func NewProjection() Projection {
	return Projection{
		Hosts:      make(map[HostKey]Host),
		Sessions:   make(map[SessionKey]Session),
		Windows:    make(map[WindowKey]Window),
		Panes:      make(map[PaneKey]Pane),
		ToolEvents: make(map[ToolEventKey]ToolEvent),
		Activity:   make(map[ActivityKey]Activity),
		Health:     make(map[HostKey]Health),
		Metadata:   make(map[string]json.RawMessage),
	}
}

type OperationKind string

const (
	OperationUpsertHost      OperationKind = "upsert-host"
	OperationRemoveHost      OperationKind = "remove-host"
	OperationUpsertSession   OperationKind = "upsert-session"
	OperationRemoveSession   OperationKind = "remove-session"
	OperationUpsertWindow    OperationKind = "upsert-window"
	OperationRemoveWindow    OperationKind = "remove-window"
	OperationUpsertPane      OperationKind = "upsert-pane"
	OperationRemovePane      OperationKind = "remove-pane"
	OperationUpsertToolEvent OperationKind = "upsert-tool-event"
	OperationRemoveToolEvent OperationKind = "remove-tool-event"
	OperationUpsertActivity  OperationKind = "upsert-activity"
	OperationRemoveActivity  OperationKind = "remove-activity"
	OperationUpsertHealth    OperationKind = "upsert-health"
	OperationRemoveHealth    OperationKind = "remove-health"
	OperationSetMetadata     OperationKind = "set-metadata"
	OperationRemoveMetadata  OperationKind = "remove-metadata"
)

// Operation is a discriminated union. Exactly one typed value is present for
// an upsert, while remove operations carry only their stable Key.
type Operation struct {
	Kind      OperationKind   `json:"kind"`
	Key       string          `json:"key,omitempty"`
	Host      *Host           `json:"host,omitempty"`
	Session   *Session        `json:"session,omitempty"`
	Window    *Window         `json:"window,omitempty"`
	Pane      *Pane           `json:"pane,omitempty"`
	ToolEvent *ToolEvent      `json:"tool_event,omitempty"`
	Activity  *Activity       `json:"activity,omitempty"`
	Health    *Health         `json:"health,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

func (op Operation) Validate() error {
	upsertValue := map[OperationKind]any{
		OperationUpsertHost: op.Host, OperationUpsertSession: op.Session,
		OperationUpsertWindow: op.Window, OperationUpsertPane: op.Pane,
		OperationUpsertToolEvent: op.ToolEvent, OperationUpsertActivity: op.Activity,
		OperationUpsertHealth: op.Health,
	}
	if value, ok := upsertValue[op.Kind]; ok {
		if isNil(value) {
			return fmt.Errorf("%s requires its typed value", op.Kind)
		}
		if op.Key != "" {
			return fmt.Errorf("%s must not carry a remove key", op.Kind)
		}
		return nil
	}
	switch op.Kind {
	case OperationRemoveHost, OperationRemoveSession, OperationRemoveWindow,
		OperationRemovePane, OperationRemoveToolEvent, OperationRemoveActivity,
		OperationRemoveHealth, OperationRemoveMetadata:
		if strings.TrimSpace(op.Key) == "" {
			return fmt.Errorf("%s requires a stable key", op.Kind)
		}
		return nil
	case OperationSetMetadata:
		if strings.TrimSpace(op.Key) == "" || len(op.Metadata) == 0 {
			return errors.New("set-metadata requires a key and JSON value")
		}
		if !json.Valid(op.Metadata) {
			return errors.New("set-metadata value must be valid JSON")
		}
		return nil
	default:
		return fmt.Errorf("unknown state operation %q", op.Kind)
	}
}

func isNil(value any) bool {
	switch v := value.(type) {
	case *Host:
		return v == nil
	case *Session:
		return v == nil
	case *Window:
		return v == nil
	case *Pane:
		return v == nil
	case *ToolEvent:
		return v == nil
	case *Activity:
		return v == nil
	case *Health:
		return v == nil
	default:
		return true
	}
}

type SnapshotEnvelope struct {
	Type          string     `json:"type"`
	SchemaVersion int        `json:"schema_version"`
	InstanceID    string     `json:"instance_id"`
	Revision      uint64     `json:"revision"`
	State         Projection `json:"state"`
}

func (e SnapshotEnvelope) Validate() error {
	if e.Type != EnvelopeSnapshot {
		return fmt.Errorf("snapshot type is %q", e.Type)
	}
	return validateEnvelopeIdentity(e.SchemaVersion, e.InstanceID)
}

type DeltaEnvelope struct {
	Type          string      `json:"type"`
	SchemaVersion int         `json:"schema_version"`
	InstanceID    string      `json:"instance_id"`
	BaseRevision  uint64      `json:"base_revision"`
	Revision      uint64      `json:"revision"`
	Operations    []Operation `json:"operations"`
}

func (e DeltaEnvelope) Validate() error {
	if e.Type != EnvelopeDelta {
		return fmt.Errorf("delta type is %q", e.Type)
	}
	if err := validateEnvelopeIdentity(e.SchemaVersion, e.InstanceID); err != nil {
		return err
	}
	if e.Revision <= e.BaseRevision {
		return errors.New("delta revision must advance base_revision")
	}
	if len(e.Operations) == 0 {
		return errors.New("delta must contain at least one operation")
	}
	for i, operation := range e.Operations {
		if err := operation.Validate(); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}
	return nil
}

type OutcomeEnvelope struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	InstanceID    string `json:"instance_id,omitempty"`
	Revision      uint64 `json:"revision,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

func validateEnvelopeIdentity(schemaVersion int, instanceID string) error {
	if schemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported state schema version %d", schemaVersion)
	}
	if strings.TrimSpace(instanceID) == "" {
		return errors.New("instance_id is required")
	}
	return nil
}
