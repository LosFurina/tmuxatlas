package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/LosFurina/tmuxatlas/pkg/peer"
)

type runtimeActionExecutor interface {
	Execute(context.Context, string, peer.SessionTarget, json.RawMessage) (peer.ActionResponse, error)
}

func handleSessionNew(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor, opts *Options) {
	var request struct {
		HostID  string `json:"host_id"`
		Session string `json:"session"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || router == nil {
		writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
		return
	}
	result, err := router.Execute(r.Context(), "new",
		peer.SessionTarget{HostID: request.HostID, Session: request.Session}, json.RawMessage(`{}`))
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	refreshLocalSessionState(opts, request.HostID)
	writeActionResponse(w, result)
}

func handleSessionRename(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor, opts *Options) {
	var request struct {
		HostID  string `json:"host_id"`
		Session string `json:"session"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.NewName == "" || router == nil {
		writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
		return
	}
	payload, _ := json.Marshal(map[string]string{"new_name": request.NewName})
	result, err := router.Execute(r.Context(), "rename",
		peer.SessionTarget{HostID: request.HostID, Session: request.Session}, payload)
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	refreshLocalSessionState(opts, request.HostID)
	writeActionResponse(w, result)
}

func handleSessionSelectWindow(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor) {
	var request struct {
		HostID  string `json:"host_id"`
		Session string `json:"session"`
		Window  int    `json:"window"`
		Pane    string `json:"pane,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || router == nil {
		writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
		return
	}
	payload, _ := json.Marshal(map[string]any{"window": request.Window, "pane": request.Pane})
	result, err := router.Execute(r.Context(), "select-window",
		peer.SessionTarget{HostID: request.HostID, Session: request.Session}, payload)
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeActionResponse(w, result)
}

func handleSessionSelectPane(w http.ResponseWriter, r *http.Request, router runtimeActionExecutor) {
	var request struct {
		HostID  string `json:"host_id"`
		Session string `json:"session"`
		Pane    string `json:"pane"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Pane == "" || router == nil {
		writeRuntimeError(w, peer.RuntimeError{Code: peer.ErrorInvalidTarget})
		return
	}
	payload, _ := json.Marshal(map[string]string{"pane": request.Pane})
	result, err := router.Execute(r.Context(), "select-pane",
		peer.SessionTarget{HostID: request.HostID, Session: request.Session}, payload)
	if err != nil {
		writeRuntimeError(w, err)
		return
	}
	writeActionResponse(w, result)
}

func refreshLocalSessionState(opts *Options, hostID string) {
	if opts == nil || opts.PeerMgr == nil || opts.Client == nil || hostID != opts.PeerMgr.LocalID() {
		return
	}
	if fresh, err := opts.Client.ListSessions(); err == nil {
		opts.StateMgr.UpdateSessions(fresh)
	}
}

func writeActionResponse(w http.ResponseWriter, result peer.ActionResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
