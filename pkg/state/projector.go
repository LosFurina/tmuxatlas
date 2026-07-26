package state

import (
	"context"
	"fmt"
	"sort"

	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

// HostSnapshot is the state producer boundary. Peer and local discovery
// managers translate their mutable runtime state into this value before
// submitting it to the canonical coordinator.
type HostSnapshot struct {
	ID          string
	DisplayName string
	Online      bool
	Local       bool
	Version     string
	Sessions    []*tmux.Session
}

// ReplaceHosts commits a complete host/session projection. The diff is based
// on a defensive coordinator snapshot; all resulting typed operations are
// committed as one revision.
func (c *Coordinator) ReplaceHosts(ctx context.Context, hosts []HostSnapshot) (CommitResult, error) {
	current, err := c.Snapshot(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	operations, err := ReconcileHosts(current.State, hosts)
	if err != nil {
		return CommitResult{}, err
	}
	return c.Commit(ctx, operations...)
}

func ReconcileHosts(current Projection, hosts []HostSnapshot) ([]Operation, error) {
	next := NewProjection()
	for _, source := range hosts {
		if source.ID == "" {
			return nil, fmt.Errorf("host ID is required")
		}
		hostKey := NewHostKey(source.ID)
		next.Hosts[hostKey] = Host{
			Key: hostKey, ID: source.ID, DisplayName: source.DisplayName,
			Online: source.Online, Local: source.Local, Version: source.Version,
		}
		for _, tmuxSession := range source.Sessions {
			if tmuxSession == nil || tmuxSession.Name == "" {
				continue
			}
			sessionKey := NewSessionKey(source.ID, tmuxSession.Name)
			next.Sessions[sessionKey] = Session{
				Key: sessionKey, HostKey: hostKey, HostID: source.ID,
				Name: tmuxSession.Name, TmuxID: tmuxSession.ID,
				Attached: tmuxSession.Attached, Created: tmuxSession.Created,
				LastActivity: tmuxSession.LastActivity,
			}
			for _, tmuxWindow := range tmuxSession.Windows {
				if tmuxWindow == nil || tmuxWindow.ID == "" {
					continue
				}
				windowKey := NewWindowKey(source.ID, tmuxSession.Name, tmuxWindow.ID)
				next.Windows[windowKey] = Window{
					Key: windowKey, SessionKey: sessionKey, TmuxID: tmuxWindow.ID,
					Name: tmuxWindow.Name, Index: tmuxWindow.Index,
					Active: tmuxWindow.Active, Layout: tmuxWindow.Layout,
				}
				for _, tmuxPane := range tmuxWindow.Panes {
					if tmuxPane == nil || tmuxPane.ID == "" {
						continue
					}
					paneKey := NewPaneKey(source.ID, tmuxSession.Name, tmuxWindow.ID, tmuxPane.ID)
					next.Panes[paneKey] = Pane{
						Key: paneKey, WindowKey: windowKey, TmuxID: tmuxPane.ID,
						Index: tmuxPane.Index, Active: tmuxPane.Active,
						Width: tmuxPane.Width, Height: tmuxPane.Height,
						CurrentCommand: tmuxPane.CurrentCommand, PID: tmuxPane.PID,
					}
				}
			}
		}
	}

	var operations []Operation
	operations = appendMapDiff(operations, current.Panes, next.Panes,
		func(key PaneKey) Operation { return Operation{Kind: OperationRemovePane, Key: string(key)} },
		func(value Pane) Operation { return Operation{Kind: OperationUpsertPane, Pane: &value} })
	operations = appendMapDiff(operations, current.Windows, next.Windows,
		func(key WindowKey) Operation { return Operation{Kind: OperationRemoveWindow, Key: string(key)} },
		func(value Window) Operation { return Operation{Kind: OperationUpsertWindow, Window: &value} })
	operations = appendMapDiff(operations, current.Sessions, next.Sessions,
		func(key SessionKey) Operation { return Operation{Kind: OperationRemoveSession, Key: string(key)} },
		func(value Session) Operation { return Operation{Kind: OperationUpsertSession, Session: &value} })
	operations = appendMapDiff(operations, current.Hosts, next.Hosts,
		func(key HostKey) Operation { return Operation{Kind: OperationRemoveHost, Key: string(key)} },
		func(value Host) Operation { return Operation{Kind: OperationUpsertHost, Host: &value} })
	return operations, nil
}

func appendMapDiff[K ~string, V comparable](
	operations []Operation,
	current map[K]V,
	next map[K]V,
	remove func(K) Operation,
	upsert func(V) Operation,
) []Operation {
	removeKeys := make([]K, 0)
	for key := range current {
		if _, ok := next[key]; !ok {
			removeKeys = append(removeKeys, key)
		}
	}
	sort.Slice(removeKeys, func(i, j int) bool { return removeKeys[i] < removeKeys[j] })
	for _, key := range removeKeys {
		operations = append(operations, remove(key))
	}

	upsertKeys := make([]K, 0)
	for key, value := range next {
		if previous, ok := current[key]; !ok || previous != value {
			upsertKeys = append(upsertKeys, key)
		}
	}
	sort.Slice(upsertKeys, func(i, j int) bool { return upsertKeys[i] < upsertKeys[j] })
	for _, key := range upsertKeys {
		operations = append(operations, upsert(next[key]))
	}
	return operations
}
