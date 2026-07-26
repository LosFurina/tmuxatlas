package peer

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestHelloNegotiationAndJSONRoundTrip(t *testing.T) {
	hello := HelloPayload{
		MinVersion: 1, MaxVersion: 2,
		Capabilities:    []string{CapabilityStateSync, CapabilityPTYControl, "future-capability"},
		BuildVersion:    "v1.2.3",
		AgentInstanceID: "agent-instance-1",
	}
	raw, err := json.Marshal(hello)
	if err != nil {
		t.Fatal(err)
	}
	var decoded HelloPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hello, decoded) {
		t.Fatalf("round trip = %#v, want %#v", decoded, hello)
	}
	ack, err := NegotiateHello(decoded, 7, "hub-v1")
	if err != nil {
		t.Fatal(err)
	}
	if ack.Version != 1 || ack.Generation != 7 || ack.AgentInstanceID != hello.AgentInstanceID {
		t.Fatalf("unexpected ack: %#v", ack)
	}
	if !reflect.DeepEqual(ack.Capabilities, []string{CapabilityStateSync, CapabilityPTYControl}) {
		t.Fatalf("capabilities = %v", ack.Capabilities)
	}
	if err := ack.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHelloValidationAndIncompatibleVersion(t *testing.T) {
	tests := []HelloPayload{
		{},
		{MinVersion: 2, MaxVersion: 1, Capabilities: []string{"x"}, BuildVersion: "v", AgentInstanceID: "id"},
		{MinVersion: 1, MaxVersion: 1, Capabilities: []string{"x", "x"}, BuildVersion: "v", AgentInstanceID: "id"},
		{MinVersion: 1, MaxVersion: 1, Capabilities: []string{"x"}, BuildVersion: "v", AgentInstanceID: "bad id"},
	}
	for _, hello := range tests {
		if err := hello.Validate(); err == nil {
			t.Fatalf("invalid hello accepted: %#v", hello)
		}
	}
	_, err := NegotiateHello(HelloPayload{
		MinVersion: 2, MaxVersion: 3, Capabilities: []string{"future"},
		BuildVersion: "v2", AgentInstanceID: "instance",
	}, 1, "hub")
	var runtimeError RuntimeError
	if !errors.As(err, &runtimeError) || runtimeError.Code != ErrorProtocolIncompatible {
		t.Fatalf("error = %v, want protocol-incompatible", err)
	}
}

func TestRuntimeEnvelopeValidation(t *testing.T) {
	now := time.Now()
	request := RuntimeRequest{
		RequestID: "request-1", Generation: 4, Deadline: now.Add(time.Second),
		Operation: "rename", Target: SessionTarget{HostID: "host-1", Session: "work"},
		Payload: json.RawMessage(`{"new_name":"next"}`),
	}
	if err := request.Validate(now); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RuntimeRequest
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.RequestID != request.RequestID {
		t.Fatalf("request round trip failed: %#v %v", decoded, err)
	}

	invalid := []RuntimeRequest{
		{Generation: 1, Deadline: now.Add(time.Second), Operation: "x", Target: request.Target, Payload: request.Payload},
		{RequestID: "r", Deadline: now.Add(time.Second), Operation: "x", Target: request.Target, Payload: request.Payload},
		{RequestID: "r", Generation: 1, Deadline: now.Add(-time.Second), Operation: "x", Target: request.Target, Payload: request.Payload},
		{RequestID: "r", Generation: 1, Deadline: now.Add(time.Second), Operation: "x", Target: SessionTarget{Session: "s"}, Payload: request.Payload},
		{RequestID: "r", Generation: 1, Deadline: now.Add(time.Second), Operation: "x", Target: request.Target, Payload: json.RawMessage(`{} {}`)},
	}
	for _, candidate := range invalid {
		if err := candidate.Validate(now); err == nil {
			t.Fatalf("invalid request accepted: %#v", candidate)
		}
	}
}

func TestRuntimeOutcomeValidation(t *testing.T) {
	if err := (RuntimeAck{RequestID: "r", Generation: 1, Accepted: true}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (RuntimeResult{RequestID: "r", Generation: 1, Result: json.RawMessage(`{"ok":true}`)}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (RuntimeError{RequestID: "r", Generation: 1, Code: ErrorNotFound}).ValidateCorrelated(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{
		RuntimeAck{RequestID: "r", Generation: 1},
		RuntimeResult{RequestID: "r", Generation: 1, Result: json.RawMessage(`bad`)},
		RuntimeError{RequestID: "r", Generation: 1, Code: "unknown"},
	} {
		switch value := invalid.(type) {
		case RuntimeAck:
			if value.Validate() == nil {
				t.Fatal("invalid ack accepted")
			}
		case RuntimeResult:
			if value.Validate() == nil {
				t.Fatal("invalid result accepted")
			}
		case RuntimeError:
			if value.ValidateCorrelated() == nil {
				t.Fatal("invalid error accepted")
			}
		}
	}
}
