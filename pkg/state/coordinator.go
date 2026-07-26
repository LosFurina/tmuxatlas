package state

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

const (
	defaultCommandBuffer    = 128
	defaultSubscriberBuffer = 64
)

var ErrCoordinatorClosed = errors.New("state coordinator is closed")

type CommitResult struct {
	Changed  bool
	Revision uint64
	Delta    *DeltaEnvelope
}

type SubscriberMessage struct {
	Delta   *DeltaEnvelope   `json:"delta,omitempty"`
	Outcome *OutcomeEnvelope `json:"outcome,omitempty"`
}

type Subscription struct {
	Snapshot SnapshotEnvelope
	C        <-chan SubscriberMessage

	cancelOnce sync.Once
	cancel     func()
}

func (s *Subscription) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancelOnce.Do(s.cancel)
}

type commandKind uint8

const (
	commandCommit commandKind = iota
	commandSnapshot
	commandSubscribe
	commandUnsubscribe
)

type command struct {
	kind       commandKind
	operations []Operation
	subID      uint64
	queueSize  int
	response   chan commandResult
}

type commandResult struct {
	commit       CommitResult
	snapshot     SnapshotEnvelope
	subscription *subscriber
	err          error
}

type subscriber struct {
	id uint64
	ch chan SubscriberMessage
}

// Coordinator owns the canonical projection. Its run goroutine is the only
// code allowed to mutate projection, revision, or the subscriber registry.
type Coordinator struct {
	instanceID string
	schema     int
	commands   chan command
	stop       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

func NewCoordinator() (*Coordinator, error) {
	instanceID, err := NewInstanceID()
	if err != nil {
		return nil, err
	}
	return NewCoordinatorWithInstanceID(instanceID), nil
}

// NewCoordinatorWithInstanceID is primarily useful for deterministic tests.
// Production callers should use NewCoordinator so each process gets a fresh
// state generation.
func NewCoordinatorWithInstanceID(instanceID string) *Coordinator {
	if instanceID == "" {
		panic("state coordinator instance ID must not be empty")
	}
	c := &Coordinator{
		instanceID: instanceID,
		schema:     SchemaVersion,
		commands:   make(chan command, defaultCommandBuffer),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go c.run()
	return c
}

func NewInstanceID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate state instance ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func (c *Coordinator) InstanceID() string {
	return c.instanceID
}

func (c *Coordinator) SchemaVersion() int {
	return c.schema
}

func (c *Coordinator) SupportsSchema(version int) bool {
	return version == c.schema
}

func (c *Coordinator) Commit(ctx context.Context, operations ...Operation) (CommitResult, error) {
	result, err := c.request(ctx, command{kind: commandCommit, operations: operations})
	return result.commit, err
}

func (c *Coordinator) Snapshot(ctx context.Context) (SnapshotEnvelope, error) {
	result, err := c.request(ctx, command{kind: commandSnapshot})
	return result.snapshot, err
}

func (c *Coordinator) Subscribe(ctx context.Context, queueSize int) (*Subscription, error) {
	if queueSize <= 0 {
		queueSize = defaultSubscriberBuffer
	}
	result, err := c.request(ctx, command{kind: commandSubscribe, queueSize: queueSize})
	if err != nil {
		return nil, err
	}
	sub := result.subscription
	return &Subscription{
		Snapshot: result.snapshot,
		C:        sub.ch,
		cancel: func() {
			// Cancellation is best effort and never blocks shutdown.
			response := make(chan commandResult, 1)
			select {
			case c.commands <- command{kind: commandUnsubscribe, subID: sub.id, response: response}:
			case <-c.done:
			}
		},
	}, nil
}

func (c *Coordinator) Close() {
	c.closeOnce.Do(func() { close(c.stop) })
	<-c.done
}

func (c *Coordinator) request(ctx context.Context, request command) (commandResult, error) {
	request.response = make(chan commandResult, 1)
	select {
	case c.commands <- request:
	case <-ctx.Done():
		return commandResult{}, ctx.Err()
	case <-c.done:
		return commandResult{}, ErrCoordinatorClosed
	}

	select {
	case result := <-request.response:
		return result, result.err
	case <-ctx.Done():
		return commandResult{}, ctx.Err()
	case <-c.done:
		return commandResult{}, ErrCoordinatorClosed
	}
}

func (c *Coordinator) run() {
	defer close(c.done)

	projection := NewProjection()
	var revision uint64
	var nextSubscriberID uint64
	subscribers := make(map[uint64]*subscriber)

	for {
		select {
		case <-c.stop:
			for id, sub := range subscribers {
				close(sub.ch)
				delete(subscribers, id)
			}
			return
		case request := <-c.commands:
			switch request.kind {
			case commandCommit:
				result, next, err := c.applyCommit(projection, revision, request.operations)
				if err == nil && result.Changed {
					projection = next
					revision = result.Revision
					for id, sub := range subscribers {
						message := SubscriberMessage{Delta: cloneDelta(result.Delta)}
						select {
						case sub.ch <- message:
						default:
							for len(sub.ch) > 0 {
								<-sub.ch
							}
							sub.ch <- SubscriberMessage{Outcome: &OutcomeEnvelope{
								Type: EnvelopeResyncRequired, SchemaVersion: c.schema,
								InstanceID: c.instanceID, Revision: revision,
								Reason: "subscriber queue overflow",
							}}
							close(sub.ch)
							delete(subscribers, id)
						}
					}
				}
				request.response <- commandResult{commit: result, err: err}
			case commandSnapshot:
				request.response <- commandResult{
					snapshot: c.snapshot(projection, revision),
				}
			case commandSubscribe:
				nextSubscriberID++
				sub := &subscriber{
					id: nextSubscriberID,
					ch: make(chan SubscriberMessage, request.queueSize),
				}
				// Registration and snapshot happen at the same point in the
				// single-writer command order.
				subscribers[sub.id] = sub
				request.response <- commandResult{
					snapshot: c.snapshot(projection, revision), subscription: sub,
				}
			case commandUnsubscribe:
				if sub, ok := subscribers[request.subID]; ok {
					delete(subscribers, request.subID)
					close(sub.ch)
				}
				request.response <- commandResult{}
			}
		}
	}
}

func (c *Coordinator) snapshot(projection Projection, revision uint64) SnapshotEnvelope {
	return SnapshotEnvelope{
		Type: EnvelopeSnapshot, SchemaVersion: c.schema,
		InstanceID: c.instanceID, Revision: revision,
		State: cloneProjection(projection),
	}
}

func (c *Coordinator) applyCommit(
	current Projection,
	revision uint64,
	operations []Operation,
) (CommitResult, Projection, error) {
	if len(operations) == 0 {
		return CommitResult{Revision: revision}, current, nil
	}
	next := cloneProjection(current)
	for index, operation := range operations {
		if err := operation.Validate(); err != nil {
			return CommitResult{Revision: revision}, current, fmt.Errorf("operation %d: %w", index, err)
		}
		if err := applyOperation(&next, operation); err != nil {
			return CommitResult{Revision: revision}, current, fmt.Errorf("operation %d: %w", index, err)
		}
	}
	if reflect.DeepEqual(current, next) {
		return CommitResult{Revision: revision}, current, nil
	}
	delta := &DeltaEnvelope{
		Type: EnvelopeDelta, SchemaVersion: c.schema, InstanceID: c.instanceID,
		BaseRevision: revision, Revision: revision + 1,
		Operations: cloneOperations(operations),
	}
	return CommitResult{Changed: true, Revision: revision + 1, Delta: delta}, next, nil
}

func applyOperation(projection *Projection, operation Operation) error {
	switch operation.Kind {
	case OperationUpsertHost:
		projection.Hosts[operation.Host.Key] = *operation.Host
	case OperationRemoveHost:
		delete(projection.Hosts, HostKey(operation.Key))
	case OperationUpsertSession:
		projection.Sessions[operation.Session.Key] = *operation.Session
	case OperationRemoveSession:
		delete(projection.Sessions, SessionKey(operation.Key))
	case OperationUpsertWindow:
		projection.Windows[operation.Window.Key] = *operation.Window
	case OperationRemoveWindow:
		delete(projection.Windows, WindowKey(operation.Key))
	case OperationUpsertPane:
		projection.Panes[operation.Pane.Key] = *operation.Pane
	case OperationRemovePane:
		delete(projection.Panes, PaneKey(operation.Key))
	case OperationUpsertToolEvent:
		projection.ToolEvents[operation.ToolEvent.Key] = *operation.ToolEvent
	case OperationRemoveToolEvent:
		delete(projection.ToolEvents, ToolEventKey(operation.Key))
	case OperationUpsertActivity:
		projection.Activity[operation.Activity.Key] = *operation.Activity
	case OperationRemoveActivity:
		delete(projection.Activity, ActivityKey(operation.Key))
	case OperationUpsertHealth:
		projection.Health[operation.Health.HostKey] = cloneHealth(*operation.Health)
	case OperationRemoveHealth:
		delete(projection.Health, HostKey(operation.Key))
	case OperationSetMetadata:
		projection.Metadata[operation.Key] = append([]byte(nil), operation.Metadata...)
	case OperationRemoveMetadata:
		delete(projection.Metadata, operation.Key)
	default:
		return fmt.Errorf("unsupported operation %q", operation.Kind)
	}
	return nil
}

func cloneProjection(source Projection) Projection {
	target := NewProjection()
	for key, value := range source.Hosts {
		target.Hosts[key] = value
	}
	for key, value := range source.Sessions {
		target.Sessions[key] = value
	}
	for key, value := range source.Windows {
		target.Windows[key] = value
	}
	for key, value := range source.Panes {
		target.Panes[key] = value
	}
	for key, value := range source.ToolEvents {
		target.ToolEvents[key] = value
	}
	for key, value := range source.Activity {
		value.Data = cloneJSONValue(value.Data)
		target.Activity[key] = value
	}
	for key, value := range source.Health {
		target.Health[key] = cloneHealth(value)
	}
	for key, value := range source.Metadata {
		target.Metadata[key] = append([]byte(nil), value...)
	}
	return target
}

func cloneHealth(source Health) Health {
	target := source
	if source.Facts != nil {
		target.Facts = make(map[string]any, len(source.Facts))
		for key, value := range source.Facts {
			target.Facts[key] = cloneJSONValue(value)
		}
	}
	return target
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneJSONValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = cloneJSONValue(item)
		}
		return result
	case []string:
		return append([]string(nil), value...)
	case []int64:
		return append([]int64(nil), value...)
	case []byte:
		return append([]byte(nil), value...)
	default:
		return value
	}
}

func cloneOperations(source []Operation) []Operation {
	target := make([]Operation, len(source))
	for index, operation := range source {
		target[index] = cloneOperation(operation)
	}
	return target
}

func cloneOperation(source Operation) Operation {
	target := source
	if source.Host != nil {
		value := *source.Host
		target.Host = &value
	}
	if source.Session != nil {
		value := *source.Session
		target.Session = &value
	}
	if source.Window != nil {
		value := *source.Window
		target.Window = &value
	}
	if source.Pane != nil {
		value := *source.Pane
		target.Pane = &value
	}
	if source.ToolEvent != nil {
		value := *source.ToolEvent
		target.ToolEvent = &value
	}
	if source.Activity != nil {
		value := *source.Activity
		value.Data = cloneJSONValue(value.Data)
		target.Activity = &value
	}
	if source.Health != nil {
		value := cloneHealth(*source.Health)
		target.Health = &value
	}
	target.Metadata = append([]byte(nil), source.Metadata...)
	return target
}

func cloneDelta(source *DeltaEnvelope) *DeltaEnvelope {
	if source == nil {
		return nil
	}
	target := *source
	target.Operations = cloneOperations(source.Operations)
	return &target
}
