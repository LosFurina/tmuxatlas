package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LosFurina/tmuxatlas/pkg/peer"
)

type fakeActionRouter struct {
	operation string
	target    peer.SessionTarget
	payload   json.RawMessage
	response  peer.ActionResponse
	err       error
}

func (router *fakeActionRouter) Execute(_ context.Context, operation string, target peer.SessionTarget, payload json.RawMessage) (peer.ActionResponse, error) {
	router.operation, router.target, router.payload = operation, target, append([]byte(nil), payload...)
	return router.response, router.err
}

func TestRuntimeAPIRequiresExplicitHostAndReturnsRemoteRenameTarget(t *testing.T) {
	invalid := httptest.NewRecorder()
	handleSessionRename(invalid, httptest.NewRequest(http.MethodPost, "/",
		bytes.NewBufferString(`{"session":"same","new_name":"renamed"}`)),
		&fakeActionRouter{err: peer.RuntimeError{Code: peer.ErrorInvalidTarget}}, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("missing host status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	result, _ := json.Marshal(map[string]any{
		"target": peer.SessionTarget{HostID: "remote", Session: "renamed"},
	})
	router := &fakeActionRouter{response: peer.ActionResponse{
		RequestID: "request", Result: result,
	}}
	response := httptest.NewRecorder()
	handleSessionRename(response, httptest.NewRequest(http.MethodPost, "/",
		bytes.NewBufferString(`{"host_id":"remote","session":"same","new_name":"renamed"}`)),
		router, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if router.operation != "rename" ||
		router.target != (peer.SessionTarget{HostID: "remote", Session: "same"}) ||
		string(router.payload) != `{"new_name":"renamed"}` {
		t.Fatalf("route operation=%q target=%#v payload=%s", router.operation, router.target, router.payload)
	}
}

func TestRuntimeAPIMapsStructuredTerminalErrors(t *testing.T) {
	for _, test := range []struct {
		code   peer.ErrorCode
		status int
	}{
		{peer.ErrorNotFound, http.StatusNotFound},
		{peer.ErrorPeerOffline, http.StatusServiceUnavailable},
		{peer.ErrorCapabilityUnsupported, http.StatusConflict},
		{peer.ErrorQueueFull, http.StatusTooManyRequests},
		{peer.ErrorTimeout, http.StatusGatewayTimeout},
	} {
		router := &fakeActionRouter{err: peer.RuntimeError{
			RequestID: "request", Generation: 2, Code: test.code,
		}}
		response := httptest.NewRecorder()
		handleSessionSelectPane(response, httptest.NewRequest(http.MethodPost, "/",
			bytes.NewBufferString(`{"host_id":"remote","session":"same","pane":"%1"}`)), router)
		if response.Code != test.status {
			t.Fatalf("%s status=%d body=%s", test.code, response.Code, response.Body.String())
		}
		var runtimeError peer.RuntimeError
		if err := json.Unmarshal(response.Body.Bytes(), &runtimeError); err != nil ||
			runtimeError.Code != test.code || runtimeError.RequestID != "request" {
			t.Fatalf("%s response=%#v err=%v", test.code, runtimeError, err)
		}
		var targetError peer.RuntimeError
		if !errors.As(router.err, &targetError) {
			t.Fatalf("%s lost structured error", test.code)
		}
	}
}
