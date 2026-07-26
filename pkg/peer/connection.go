package peer

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrPeerOffline     = errors.New("peer offline")
	ErrQueueFull       = errors.New("peer send queue full")
	ErrStaleGeneration = errors.New("stale peer generation")
)

type PeerConnection struct {
	HostID        string
	Generation    uint64
	Capabilities  map[string]struct{}
	AgentInstance string

	ctx      context.Context
	cancel   context.CancelFunc
	send     chan *Message
	writer   func(*Message) error
	closeFn  func() error
	start    sync.Once
	close    sync.Once
	stateMu  sync.Mutex
	closed   bool
	writerWG sync.WaitGroup
	requests *RequestTracker
	ptyMu    sync.Mutex
	ptys     map[string]*PTYOwner
}

func newPeerConnection(parent context.Context, hostID string, generation uint64, capabilities []string,
	agentInstance string, queueSize int, writer func(*Message) error, closeFn func() error) *PeerConnection {
	ctx, cancel := context.WithCancel(parent)
	capabilitySet := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capabilitySet[capability] = struct{}{}
	}
	return &PeerConnection{
		HostID: hostID, Generation: generation, Capabilities: capabilitySet,
		AgentInstance: agentInstance, ctx: ctx, cancel: cancel,
		send: make(chan *Message, queueSize), writer: writer, closeFn: closeFn,
		requests: NewRequestTracker(), ptys: make(map[string]*PTYOwner),
	}
}

func (connection *PeerConnection) Start() {
	connection.start.Do(func() {
		connection.writerWG.Add(1)
		go func() {
			defer connection.writerWG.Done()
			for {
				select {
				case <-connection.ctx.Done():
					return
				case message := <-connection.send:
					if err := connection.writer(message); err != nil {
						connection.Close()
						return
					}
				}
			}
		}()
	})
}

func (connection *PeerConnection) Send(ctx context.Context, message *Message) error {
	if message == nil {
		return errors.New("message is nil")
	}
	connection.stateMu.Lock()
	defer connection.stateMu.Unlock()
	if connection.closed {
		return ErrPeerOffline
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case connection.send <- message:
		return nil
	default:
		return ErrQueueFull
	}
}

func (connection *PeerConnection) Supports(capability string) bool {
	_, ok := connection.Capabilities[capability]
	return ok
}

func (connection *PeerConnection) Close() {
	connection.CloseWith(ErrorPeerOffline)
}

func (connection *PeerConnection) CloseWith(code ErrorCode) {
	connection.close.Do(func() {
		connection.stateMu.Lock()
		connection.closed = true
		connection.stateMu.Unlock()
		connection.cancel()
		connection.requests.FailAll(connection.Generation, code)
		connection.ptyMu.Lock()
		owners := make([]*PTYOwner, 0, len(connection.ptys))
		for _, owner := range connection.ptys {
			owners = append(owners, owner)
		}
		connection.ptys = make(map[string]*PTYOwner)
		connection.ptyMu.Unlock()
		for _, owner := range owners {
			owner.Teardown(string(code))
		}
		for _, owner := range owners {
			owner.Wait()
		}
		if connection.closeFn != nil {
			_ = connection.closeFn()
		}
	})
}

func (connection *PeerConnection) ownPTY(owner *PTYOwner) error {
	connection.stateMu.Lock()
	closed := connection.closed
	connection.stateMu.Unlock()
	if closed {
		return ErrPeerOffline
	}
	connection.ptyMu.Lock()
	defer connection.ptyMu.Unlock()
	connection.ptys[owner.StreamID] = owner
	return nil
}

func (connection *PeerConnection) releasePTY(streamID string) {
	connection.ptyMu.Lock()
	delete(connection.ptys, streamID)
	connection.ptyMu.Unlock()
}

func (connection *PeerConnection) Wait() {
	connection.writerWG.Wait()
}

func (connection *PeerConnection) Done() <-chan struct{} {
	return connection.ctx.Done()
}
