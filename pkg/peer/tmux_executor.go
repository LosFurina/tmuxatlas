package peer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LosFurina/tmuxatlas/pkg/tmux"
)

type tmuxRuntimeExecutor struct {
	client *tmux.Client
}

func (executor tmuxRuntimeExecutor) Execute(_ context.Context, operation string, target SessionTarget, payload json.RawMessage) (json.RawMessage, error) {
	if executor.client == nil {
		return nil, fmt.Errorf("tmux client unavailable")
	}
	switch operation {
	case "new":
		if err := executor.client.NewSession(target.Session); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"target": target})
	case "rename":
		var params struct {
			NewName string `json:"new_name"`
		}
		if json.Unmarshal(payload, &params) != nil || params.NewName == "" {
			return nil, fmt.Errorf("new_name is required")
		}
		if err := executor.client.RenameSession(target.Session, params.NewName); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"target": SessionTarget{HostID: target.HostID, Session: params.NewName},
		})
	case "select-window":
		var params struct {
			Window int    `json:"window"`
			Pane   string `json:"pane,omitempty"`
		}
		if json.Unmarshal(payload, &params) != nil {
			return nil, fmt.Errorf("invalid select-window payload")
		}
		if err := executor.client.SelectWindow(target.Session, fmt.Sprintf("%d", params.Window)); err != nil {
			return nil, err
		}
		if params.Pane != "" {
			if err := executor.client.SelectPane(params.Pane); err != nil {
				return nil, err
			}
		}
		return json.RawMessage(`{"ok":true}`), nil
	case "select-pane":
		var params struct {
			Pane string `json:"pane"`
		}
		if json.Unmarshal(payload, &params) != nil || params.Pane == "" {
			return nil, fmt.Errorf("pane is required")
		}
		if err := executor.client.SelectPane(params.Pane); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"ok":true}`), nil
	case "new-window":
		index, err := executor.client.NewWindow(target.Session)
		if err != nil {
			return nil, err
		}
		result, _ := json.Marshal(map[string]any{
			"target": target,
			"window": index,
		})
		return result, nil
	case "mouse":
		var params struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(payload, &params) != nil ||
			(params.Value != "on" && params.Value != "off") {
			return nil, fmt.Errorf("value must be 'on' or 'off'")
		}
		if err := executor.client.SetMouse(target.Session, params.Value == "on"); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"ok":true}`), nil

	default:
		return nil, fmt.Errorf("unsupported operation")
	}
}
