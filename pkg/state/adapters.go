package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func NewToolEventKey(hostID, session string, window int, pane, tool string) ToolEventKey {
	return ToolEventKey(fmt.Sprintf(
		"%s/tool/%s/%s/%s/%s",
		NewSessionKey(hostID, session),
		keyPart(strconv.Itoa(window)),
		keyPart(pane),
		keyPart(tool),
		keyPart(hostID),
	))
}

func NewActivityKey(hostID, session string) ActivityKey {
	return ActivityKey(fmt.Sprintf("%s/activity", NewSessionKey(hostID, session)))
}

// ApplyToolEvent adapts the latest tracker event into one canonical mutation.
// Active and completed are clearing transitions in the existing Tracker.
func (c *Coordinator) ApplyToolEvent(ctx context.Context, event *toolevents.Event, localHostID string) (CommitResult, error) {
	if event == nil {
		return CommitResult{}, errors.New("tool event is nil")
	}
	hostID := event.Host
	if hostID == "" {
		hostID = localHostID
	}
	if hostID == "" {
		return CommitResult{}, errors.New("tool event host ID is required")
	}
	key := NewToolEventKey(hostID, event.Session, event.Window, event.Pane, string(event.Tool))
	if (event.Status == toolevents.StatusActive && !event.AutoDetected) ||
		event.Status == toolevents.StatusCompleted {
		return c.Commit(ctx, Operation{Kind: OperationRemoveToolEvent, Key: string(key)})
	}
	value := ToolEvent{
		Key: key, HostID: hostID, Session: event.Session,
		Window: strconv.Itoa(event.Window), Pane: event.Pane,
		Tool: string(event.Tool), Status: string(event.Status),
		Message: event.Message, Timestamp: event.Timestamp,
		AutoDetected: event.AutoDetected,
	}
	return c.Commit(ctx, Operation{Kind: OperationUpsertToolEvent, ToolEvent: &value})
}

// ReplaceActivity projects all currently known activity samples as one
// revision, including removal of samples no longer reported by producers.
func (c *Coordinator) ReplaceActivity(
	ctx context.Context,
	snapshots []*activity.Snapshot,
	localHostID string,
) (CommitResult, error) {
	current, err := c.Snapshot(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	next := make(map[ActivityKey]Activity)
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		hostID := snapshot.Host
		if hostID == "" {
			hostID = localHostID
		}
		if hostID == "" || snapshot.SessionName == "" {
			continue
		}
		key := NewActivityKey(hostID, snapshot.SessionName)
		next[key] = Activity{
			Key: key, HostID: hostID, Session: snapshot.SessionName,
			Data: map[string]any{
				"idle_seconds": snapshot.IdleSeconds,
				"sparkline":    append([]int64(nil), snapshot.Sparkline...),
				"total_bytes":  snapshot.TotalBytes,
			},
		}
	}
	var operations []Operation
	removeKeys := sortedActivityKeys(current.State.Activity)
	for _, key := range removeKeys {
		if _, ok := next[key]; !ok {
			operations = append(operations, Operation{Kind: OperationRemoveActivity, Key: string(key)})
		}
	}
	upsertKeys := sortedActivityKeys(next)
	for _, key := range upsertKeys {
		value := next[key]
		if previous, ok := current.State.Activity[key]; !ok || !reflect.DeepEqual(previous, value) {
			value := value
			operations = append(operations, Operation{Kind: OperationUpsertActivity, Activity: &value})
		}
	}
	return c.Commit(ctx, operations...)
}

func sortedActivityKeys(values map[ActivityKey]Activity) []ActivityKey {
	keys := make([]ActivityKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortActivityKeys(keys)
	return keys
}

func sortActivityKeys(keys []ActivityKey) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

func (c *Coordinator) SetMetadata(ctx context.Context, key string, value any) (CommitResult, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return CommitResult{}, fmt.Errorf("marshal state metadata %q: %w", key, err)
	}
	return c.Commit(ctx, Operation{Kind: OperationSetMetadata, Key: key, Metadata: raw})
}

func (c *Coordinator) UpsertHealth(ctx context.Context, health Health) (CommitResult, error) {
	if health.HostKey == "" {
		return CommitResult{}, errors.New("health host key is required")
	}
	return c.Commit(ctx, Operation{Kind: OperationUpsertHealth, Health: &health})
}

func (c *Coordinator) ClearToolEvents(
	ctx context.Context,
	hostID string,
	session string,
	window int,
	pane string,
) (CommitResult, error) {
	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	var operations []Operation
	for key, event := range snapshot.State.ToolEvents {
		if session != "" && (event.HostID != hostID || event.Session != session ||
			event.Window != strconv.Itoa(window) || event.Pane != pane) {
			continue
		}
		operations = append(operations, Operation{Kind: OperationRemoveToolEvent, Key: string(key)})
	}
	return c.Commit(ctx, operations...)
}
