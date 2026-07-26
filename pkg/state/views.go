package state

import (
	"sort"
	"strconv"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/activity"
	"github.com/LosFurina/tmuxatlas/pkg/tmux"
	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

type HostView struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Version  string                 `json:"version,omitempty"`
	Local    bool                   `json:"local,omitempty"`
	Online   bool                   `json:"online"`
	Sessions []*tmux.Session        `json:"sessions"`
	Stats    map[string]interface{} `json:"stats,omitempty"`
	LastSeen time.Time              `json:"last_seen"`
}

func (p Projection) SessionViews() []*tmux.Session {
	keys := make([]SessionKey, 0, len(p.Sessions))
	for key := range p.Sessions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	result := make([]*tmux.Session, 0, len(keys))
	for _, key := range keys {
		source := p.Sessions[key]
		host := p.Hosts[source.HostKey]
		session := &tmux.Session{
			ID: source.TmuxID, Name: source.Name,
			Host: source.HostID, HostName: host.DisplayName, HostOnline: host.Online,
			Created: source.Created, Attached: source.Attached,
			LastActivity: source.LastActivity, Windows: []*tmux.Window{},
		}
		windowKeys := make([]WindowKey, 0)
		for windowKey, window := range p.Windows {
			if window.SessionKey == key {
				windowKeys = append(windowKeys, windowKey)
			}
		}
		sort.Slice(windowKeys, func(i, j int) bool {
			left, right := p.Windows[windowKeys[i]], p.Windows[windowKeys[j]]
			if left.Index != right.Index {
				return left.Index < right.Index
			}
			return windowKeys[i] < windowKeys[j]
		})
		for _, windowKey := range windowKeys {
			sourceWindow := p.Windows[windowKey]
			window := &tmux.Window{
				ID: sourceWindow.TmuxID, SessionID: source.TmuxID,
				Name: sourceWindow.Name, Index: sourceWindow.Index,
				Active: sourceWindow.Active, Layout: sourceWindow.Layout,
				Panes: []*tmux.Pane{},
			}
			paneKeys := make([]PaneKey, 0)
			for paneKey, pane := range p.Panes {
				if pane.WindowKey == windowKey {
					paneKeys = append(paneKeys, paneKey)
				}
			}
			sort.Slice(paneKeys, func(i, j int) bool {
				left, right := p.Panes[paneKeys[i]], p.Panes[paneKeys[j]]
				if left.Index != right.Index {
					return left.Index < right.Index
				}
				return paneKeys[i] < paneKeys[j]
			})
			for _, paneKey := range paneKeys {
				sourcePane := p.Panes[paneKey]
				window.Panes = append(window.Panes, &tmux.Pane{
					ID: sourcePane.TmuxID, WindowID: sourceWindow.TmuxID,
					SessionID: source.TmuxID, Index: sourcePane.Index,
					Active: sourcePane.Active, Width: sourcePane.Width,
					Height: sourcePane.Height, CurrentCommand: sourcePane.CurrentCommand,
					PID: sourcePane.PID,
				})
			}
			session.Windows = append(session.Windows, window)
		}
		result = append(result, session)
	}
	return result
}

func (p Projection) HostViews() []HostView {
	sessions := p.SessionViews()
	byHost := make(map[string][]*tmux.Session)
	for _, session := range sessions {
		byHost[session.Host] = append(byHost[session.Host], session)
	}
	keys := make([]HostKey, 0, len(p.Hosts))
	for key := range p.Hosts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]HostView, 0, len(keys))
	for _, key := range keys {
		host := p.Hosts[key]
		result = append(result, HostView{
			ID: host.ID, Name: host.DisplayName, Version: host.Version,
			Local: host.Local, Online: host.Online, Sessions: byHost[host.ID],
			LastSeen: host.LastSeen,
		})
	}
	return result
}

func (p Projection) ToolEventViews() []*toolevents.Event {
	keys := make([]ToolEventKey, 0, len(p.ToolEvents))
	for key := range p.ToolEvents {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]*toolevents.Event, 0, len(keys))
	for _, key := range keys {
		event := p.ToolEvents[key]
		window, _ := strconv.Atoi(event.Window)
		result = append(result, &toolevents.Event{
			Tool: toolevents.Tool(event.Tool), Status: toolevents.Status(event.Status),
			Host: event.HostID, Session: event.Session, Window: window,
			Pane: event.Pane, Message: event.Message, Timestamp: event.Timestamp,
			AutoDetected: event.AutoDetected,
		})
	}
	return result
}

func (p Projection) ActivityViews() []*activity.Snapshot {
	keys := make([]ActivityKey, 0, len(p.Activity))
	for key := range p.Activity {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]*activity.Snapshot, 0, len(keys))
	for _, key := range keys {
		value := p.Activity[key]
		snapshot := &activity.Snapshot{Host: value.HostID, SessionName: value.Session}
		if data, ok := value.Data.(map[string]any); ok {
			snapshot.IdleSeconds, _ = data["idle_seconds"].(float64)
			snapshot.TotalBytes, _ = data["total_bytes"].(int64)
			snapshot.Sparkline = int64Slice(data["sparkline"])
		}
		result = append(result, snapshot)
	}
	return result
}

func int64Slice(value any) []int64 {
	switch values := value.(type) {
	case []int64:
		return append([]int64(nil), values...)
	case []any:
		result := make([]int64, 0, len(values))
		for _, value := range values {
			switch number := value.(type) {
			case int64:
				result = append(result, number)
			case float64:
				result = append(result, int64(number))
			}
		}
		return result
	default:
		return nil
	}
}
