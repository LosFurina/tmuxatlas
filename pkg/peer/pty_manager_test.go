package peer

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/identity"
)

type fakePTYDevice struct {
	closeOnce sync.Once
	closed    chan struct{}
	resized   chan [2]uint16
	input     chan []byte
}

func newFakePTYDevice() *fakePTYDevice {
	return &fakePTYDevice{
		closed: make(chan struct{}), resized: make(chan [2]uint16, 1), input: make(chan []byte, 1),
	}
}

func (device *fakePTYDevice) Read([]byte) (int, error) {
	<-device.closed
	return 0, io.EOF
}

func (device *fakePTYDevice) Write(data []byte) (int, error) {
	select {
	case device.input <- append([]byte(nil), data...):
	default:
	}
	return len(data), nil
}

func (device *fakePTYDevice) Resize(cols, rows uint16) error {
	device.resized <- [2]uint16{cols, rows}
	return nil
}

func (device *fakePTYDevice) Close() {
	device.closeOnce.Do(func() { close(device.closed) })
}

func TestAgentPTYRelayAppliesResizeAndInputToBoundDevice(t *testing.T) {
	attached := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("host") != "agent-host" ||
			request.URL.Query().Get("session") != "work" ||
			request.URL.Query().Get("generation") != "7" ||
			request.URL.Query().Get("token") != "attach-token" {
			http.Error(w, "wrong binding", http.StatusBadRequest)
			return
		}
		conn, err := upgrader.Upgrade(w, request, nil)
		if err == nil {
			attached <- conn
		}
	}))
	defer server.Close()

	node, _ := identity.Generate("agent")
	manager := NewManager(node, nil, nil)
	manager.localID = "agent-host"
	client := &Client{
		hubURL:  strings.Replace(server.URL, "http://", "ws://", 1),
		peerMgr: manager, runtimeGeneration: 7,
	}
	device := newFakePTYDevice()
	ptyManager := NewPTYManager("fake-tmux", nil, client)
	ptyManager.openPTY = func(string, string, uint16, uint16) (ptyDevice, error) {
		return device, nil
	}
	request := PTYOpenPayload{
		StreamID: "stream", AttachToken: "attach-token", Generation: 7,
		Target: SessionTarget{HostID: "agent-host", Session: "work"}, Cols: 80, Rows: 24,
	}
	done := make(chan struct{})
	go func() {
		ptyManager.Open(request)
		close(done)
	}()
	var hubConn *websocket.Conn
	select {
	case hubConn = <-attached:
	case <-time.After(time.Second):
		t.Fatal("Agent did not attach data channel")
	}
	resize, _ := EncodePTYControlFrame(PTYControlFrame{
		Version: PTYFrameVersion, Type: "resize", Sequence: 1, Cols: 144, Rows: 50,
	})
	if err := hubConn.WriteMessage(websocket.TextMessage, resize); err != nil {
		t.Fatal(err)
	}
	input, _ := EncodePTYDataFrame(PTYDataInput, 1, []byte("typed"))
	if err := hubConn.WriteMessage(websocket.BinaryMessage, input); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-device.resized:
		if size != [2]uint16{144, 50} {
			t.Fatalf("resize=%v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("remote resize did not reach PTY")
	}
	select {
	case data := <-device.input:
		if string(data) != "typed" {
			t.Fatalf("input=%q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("remote input did not reach PTY")
	}
	_ = hubConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Agent PTY relay leaked after disconnect")
	}
	select {
	case <-device.closed:
	default:
		t.Fatal("PTY device was not closed")
	}
}

func TestAgentPTYRejectsStaleGenerationBeforeCreatingDevice(t *testing.T) {
	node, _ := identity.Generate("agent")
	manager := NewManager(node, nil, nil)
	manager.localID = "agent-host"
	client := &Client{peerMgr: manager, runtimeGeneration: 8}
	ptyManager := NewPTYManager("fake", nil, client)
	called := false
	ptyManager.openPTY = func(string, string, uint16, uint16) (ptyDevice, error) {
		called = true
		return newFakePTYDevice(), nil
	}
	ptyManager.Open(PTYOpenPayload{
		StreamID: "stream", AttachToken: "token", Generation: 7,
		Target: SessionTarget{HostID: "agent-host", Session: "work"}, Cols: 80, Rows: 24,
	})
	if called {
		t.Fatal("stale generation created a PTY device")
	}
}

func TestPTYInputDiagnosticLogsOnlyEnabledWheelPayloads(t *testing.T) {
	logger := logrus.StandardLogger()
	previousOutput := logger.Out
	previousFormatter := logger.Formatter
	previousLevel := logger.Level
	t.Cleanup(func() {
		logger.SetOutput(previousOutput)
		logger.SetFormatter(previousFormatter)
		logger.SetLevel(previousLevel)
	})

	var output bytes.Buffer
	logger.SetOutput(&output)
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logger.SetLevel(logrus.InfoLevel)

	t.Setenv("TMUXATLAS_DEBUG_PTY_INPUT", "")
	logPTYInputDiagnostic("work", []byte("\x1b[<64;1;1M"))
	if output.Len() != 0 {
		t.Fatalf("diagnostic logged while disabled: %q", output.String())
	}

	t.Setenv("TMUXATLAS_DEBUG_PTY_INPUT", "1")
	logPTYInputDiagnostic("work", []byte("typed"))
	if output.Len() != 0 {
		t.Fatalf("diagnostic logged non-wheel input: %q", output.String())
	}

	logPTYInputDiagnostic("work", []byte("\x1b[<64;1;1M"))
	if !strings.Contains(output.String(), "PTY input contains SGR mouse wheel sequence") {
		t.Fatalf("diagnostic did not log wheel input: %q", output.String())
	}
}
