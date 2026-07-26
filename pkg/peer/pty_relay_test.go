package peer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func websocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
	serverSide := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err == nil {
			serverSide <- conn
		}
	}))
	t.Cleanup(server.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	serverConn := <-serverSide
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverConn.Close()
	})
	return client, serverConn
}

func TestPTYOwnerRejectsMismatchedLateAttachAndTeardownIsIdempotent(t *testing.T) {
	relay := NewPTYRelay()
	connection := newPeerConnection(t.Context(), "host", 3, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	owner, err := relay.RegisterPending(connection, SessionTarget{HostID: "host", Session: "work"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if relay.CompletePending(owner.StreamID, "other", 3, owner.AttachToken, "work", nil) ||
		relay.CompletePending(owner.StreamID, "host", 2, owner.AttachToken, "work", nil) ||
		relay.CompletePending(owner.StreamID, "host", 3, "wrong", "work", nil) ||
		relay.CompletePending(owner.StreamID, "host", 3, owner.AttachToken, "other", nil) {
		t.Fatal("mismatched PTY attachment was accepted")
	}
	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			owner.Teardown("concurrent-close")
		}()
	}
	wait.Wait()
	if relay.CompletePending(owner.StreamID, "host", 3, owner.AttachToken, "work", nil) {
		t.Fatal("late PTY attachment revived a closed stream")
	}
	select {
	case <-owner.ctx.Done():
	default:
		t.Fatal("owner context remains active")
	}
}

func TestPTYBridgeFramesInputOutputAndResizeOnBoundStream(t *testing.T) {
	browserClient, browserServer := websocketPair(t)
	agentClient, agentServer := websocketPair(t)
	relay := NewPTYRelay()
	connection := newPeerConnection(t.Context(), "host", 9, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	owner, err := relay.RegisterPending(connection,
		SessionTarget{HostID: "host", Session: "target"}, browserServer)
	if err != nil {
		t.Fatal(err)
	}
	if !relay.CompletePending(owner.StreamID, "host", 9, owner.AttachToken, "target", agentServer) {
		t.Fatal("valid PTY attachment rejected")
	}
	bridgeDone := make(chan struct{})
	go func() {
		owner.Bridge()
		close(bridgeDone)
	}()

	input := []byte{0, 0xff, 'a'}
	if err := browserClient.WriteMessage(websocket.BinaryMessage, input); err != nil {
		t.Fatal(err)
	}
	messageType, encodedInput, err := agentClient.ReadMessage()
	if err != nil || messageType != websocket.BinaryMessage {
		t.Fatalf("agent input type=%d err=%v", messageType, err)
	}
	inputFrame, err := DecodePTYDataFrame(encodedInput)
	if err != nil || inputFrame.Direction != PTYDataInput || string(inputFrame.Payload) != string(input) {
		t.Fatalf("input frame=%#v err=%v", inputFrame, err)
	}

	if err := browserClient.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"resize","cols":132,"rows":43}`)); err != nil {
		t.Fatal(err)
	}
	messageType, resizeData, err := agentClient.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		t.Fatalf("resize type=%d err=%v", messageType, err)
	}
	resize, err := DecodePTYControlFrame(resizeData)
	if err != nil || resize.Type != "resize" || resize.Cols != 132 || resize.Rows != 43 {
		t.Fatalf("resize=%#v err=%v", resize, err)
	}

	output, _ := EncodePTYDataFrame(PTYDataOutput, 1, []byte("only-this-browser"))
	if err := agentClient.WriteMessage(websocket.BinaryMessage, output); err != nil {
		t.Fatal(err)
	}
	messageType, browserOutput, err := browserClient.ReadMessage()
	if err != nil || messageType != websocket.BinaryMessage || string(browserOutput) != "only-this-browser" {
		t.Fatalf("browser output=%q type=%d err=%v", browserOutput, messageType, err)
	}
	_ = browserClient.Close()
	select {
	case <-bridgeDone:
	case <-time.After(time.Second):
		t.Fatal("bridge leaked after browser disconnect")
	}
}

func TestControlGenerationCloseTearsDownOwnedPTY(t *testing.T) {
	relay := NewPTYRelay()
	connection := newPeerConnection(t.Context(), "host", 4, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	owner, err := relay.RegisterPending(connection, SessionTarget{HostID: "host", Session: "idle"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	select {
	case <-owner.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("PTY owner survived control generation close")
	}
}

func TestPTYOwnerRejectsAttachmentAfterPendingDeadline(t *testing.T) {
	relay := NewPTYRelay()
	relay.attachTimeout = 20 * time.Millisecond
	connection := newPeerConnection(t.Context(), "host", 5, RuntimeCapabilities, "instance", 1,
		func(*Message) error { return nil }, nil)
	owner, err := relay.RegisterPending(connection, SessionTarget{HostID: "host", Session: "late"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending PTY did not expire")
	}
	if relay.CompletePending(owner.StreamID, owner.HostID, owner.Generation,
		owner.AttachToken, owner.Target.Session, nil) {
		t.Fatal("late attachment was accepted")
	}
}
