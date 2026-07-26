package peer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
)

const (
	RuntimeProtocolMin = 1
	RuntimeProtocolMax = 1

	CapabilityStateSync      = "state-sync"
	CapabilitySessionActions = "session-actions"
	CapabilityRequestResults = "request-results"
	CapabilityPTYControl     = "pty-control"

	maxRuntimeIDLength = 128
	maxOperationLength = 64
	maxBuildLength     = 128
	maxPayloadSize     = 64 << 10
)

var RuntimeCapabilities = []string{
	CapabilityStateSync,
	CapabilitySessionActions,
	CapabilityRequestResults,
	CapabilityPTYControl,
}

type SessionTarget struct {
	HostID  string `json:"host_id"`
	Session string `json:"session"`
}

func (target SessionTarget) Validate() error {
	if err := validateRuntimeID("host_id", target.HostID); err != nil {
		return err
	}
	if err := validateRuntimeID("session", target.Session); err != nil {
		return err
	}
	return nil
}

type HelloPayload struct {
	MinVersion      int      `json:"min_version"`
	MaxVersion      int      `json:"max_version"`
	Capabilities    []string `json:"capabilities"`
	BuildVersion    string   `json:"build_version"`
	AgentInstanceID string   `json:"agent_instance_id"`
}

func (hello HelloPayload) Validate() error {
	if hello.MinVersion <= 0 || hello.MaxVersion < hello.MinVersion {
		return errors.New("invalid protocol version range")
	}
	if err := validateCapabilities(hello.Capabilities); err != nil {
		return err
	}
	if err := validateBoundedText("build_version", hello.BuildVersion, maxBuildLength); err != nil {
		return err
	}
	return validateRuntimeID("agent_instance_id", hello.AgentInstanceID)
}

type HelloAckPayload struct {
	Version         int      `json:"version"`
	Capabilities    []string `json:"capabilities"`
	BuildVersion    string   `json:"build_version"`
	Generation      uint64   `json:"generation"`
	AgentInstanceID string   `json:"agent_instance_id"`
}

func (ack HelloAckPayload) Validate() error {
	if ack.Version < RuntimeProtocolMin || ack.Version > RuntimeProtocolMax {
		return errors.New("unsupported selected protocol version")
	}
	if ack.Generation == 0 {
		return errors.New("connection generation is required")
	}
	if err := validateCapabilities(ack.Capabilities); err != nil {
		return err
	}
	if err := validateBoundedText("build_version", ack.BuildVersion, maxBuildLength); err != nil {
		return err
	}
	return validateRuntimeID("agent_instance_id", ack.AgentInstanceID)
}

func NegotiateHello(hello HelloPayload, generation uint64, hubBuild string) (HelloAckPayload, error) {
	if err := hello.Validate(); err != nil {
		return HelloAckPayload{}, err
	}
	minVersion := max(hello.MinVersion, RuntimeProtocolMin)
	maxVersion := min(hello.MaxVersion, RuntimeProtocolMax)
	if minVersion > maxVersion {
		return HelloAckPayload{}, RuntimeError{Code: ErrorProtocolIncompatible, Message: "no common runtime protocol version"}
	}
	return HelloAckPayload{
		Version:         maxVersion,
		Capabilities:    intersectCapabilities(hello.Capabilities, RuntimeCapabilities),
		BuildVersion:    hubBuild,
		Generation:      generation,
		AgentInstanceID: hello.AgentInstanceID,
	}, nil
}

type RuntimeRequest struct {
	RequestID  string          `json:"request_id"`
	Generation uint64          `json:"connection_generation"`
	Deadline   time.Time       `json:"deadline"`
	Operation  string          `json:"operation"`
	Target     SessionTarget   `json:"target"`
	Payload    json.RawMessage `json:"payload"`
}

func (request RuntimeRequest) Validate(now time.Time) error {
	if err := validateRuntimeID("request_id", request.RequestID); err != nil {
		return err
	}
	if request.Generation == 0 {
		return errors.New("connection generation is required")
	}
	if request.Deadline.IsZero() || !request.Deadline.After(now) {
		return errors.New("request deadline is missing or expired")
	}
	if err := validateBoundedText("operation", request.Operation, maxOperationLength); err != nil {
		return err
	}
	if err := request.Target.Validate(); err != nil {
		return fmt.Errorf("invalid target: %w", err)
	}
	if !singleJSONObject(request.Payload) {
		return errors.New("payload must be one bounded JSON object")
	}
	return nil
}

type RuntimeAck struct {
	RequestID  string `json:"request_id"`
	Generation uint64 `json:"connection_generation"`
	Accepted   bool   `json:"accepted"`
}

func (ack RuntimeAck) Validate() error {
	if err := validateRuntimeID("request_id", ack.RequestID); err != nil {
		return err
	}
	if ack.Generation == 0 || !ack.Accepted {
		return errors.New("accepted ack requires generation and accepted=true")
	}
	return nil
}

type RuntimeResult struct {
	RequestID  string          `json:"request_id"`
	Generation uint64          `json:"connection_generation"`
	Result     json.RawMessage `json:"result"`
}

func (result RuntimeResult) Validate() error {
	if err := validateRuntimeID("request_id", result.RequestID); err != nil {
		return err
	}
	if result.Generation == 0 {
		return errors.New("result generation is required")
	}
	if len(result.Result) == 0 || len(result.Result) > maxPayloadSize || !json.Valid(result.Result) {
		return errors.New("result must be bounded valid JSON")
	}
	return nil
}

type ErrorCode string

const (
	ErrorInvalidTarget         ErrorCode = "invalid-target"
	ErrorNotFound              ErrorCode = "not-found"
	ErrorPeerOffline           ErrorCode = "peer-offline"
	ErrorPeerRevoked           ErrorCode = "peer-revoked"
	ErrorProtocolIncompatible  ErrorCode = "protocol-incompatible"
	ErrorCapabilityUnsupported ErrorCode = "capability-unsupported"
	ErrorQueueFull             ErrorCode = "queue-full"
	ErrorTimeout               ErrorCode = "timeout"
	ErrorExecutionFailed       ErrorCode = "execution-failed"
	ErrorExecutionUnknown      ErrorCode = "execution-unknown"
	ErrorRequestConflict       ErrorCode = "request-conflict"
	ErrorResourceExhausted     ErrorCode = "resource-exhausted"
	ErrorStaleGeneration       ErrorCode = "stale-generation"
)

type RuntimeError struct {
	RequestID  string    `json:"request_id,omitempty"`
	Generation uint64    `json:"connection_generation,omitempty"`
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message,omitempty"`
}

func (runtimeError RuntimeError) Error() string {
	if runtimeError.Message != "" {
		return string(runtimeError.Code) + ": " + runtimeError.Message
	}
	return string(runtimeError.Code)
}

func (runtimeError RuntimeError) ValidateCorrelated() error {
	if err := validateRuntimeID("request_id", runtimeError.RequestID); err != nil {
		return err
	}
	if runtimeError.Generation == 0 {
		return errors.New("error generation is required")
	}
	return runtimeError.validateCode()
}

func (runtimeError RuntimeError) validateCode() error {
	switch runtimeError.Code {
	case ErrorInvalidTarget, ErrorNotFound, ErrorPeerOffline, ErrorPeerRevoked,
		ErrorProtocolIncompatible, ErrorCapabilityUnsupported, ErrorQueueFull,
		ErrorTimeout, ErrorExecutionFailed, ErrorExecutionUnknown,
		ErrorRequestConflict, ErrorResourceExhausted, ErrorStaleGeneration:
	default:
		return errors.New("unknown runtime error code")
	}
	return validateBoundedOptionalText("message", runtimeError.Message, 256)
}

func validateCapabilities(capabilities []string) error {
	if len(capabilities) > 32 {
		return errors.New("too many capabilities")
	}
	seen := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if err := validateBoundedText("capability", capability, 64); err != nil {
			return err
		}
		if _, exists := seen[capability]; exists {
			return errors.New("duplicate capability")
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func intersectCapabilities(left, right []string) []string {
	allowed := make(map[string]struct{}, len(right))
	for _, value := range right {
		allowed[value] = struct{}{}
	}
	result := make([]string, 0, len(left))
	for _, value := range left {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func validateRuntimeID(field, value string) error {
	if err := validateBoundedText(field, value, maxRuntimeIDLength); err != nil {
		return err
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%s contains whitespace or control characters", field)
		}
	}
	return nil
}

func validateBoundedText(field, value string, limit int) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	return validateBoundedOptionalText(field, value, limit)
}

func validateBoundedOptionalText(field, value string, limit int) error {
	if len(value) > limit || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s exceeds its bound or contains NUL", field)
	}
	return nil
}

func singleJSONObject(value []byte) bool {
	if len(value) == 0 || len(value) > maxPayloadSize {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil || object == nil {
		return false
	}
	var trailing json.RawMessage
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}
