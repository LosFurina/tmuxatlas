package voice

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEnabled(t *testing.T) {
	tests := map[string]bool{
		"": false, "off": false, "no": false, "0": false, "on": true, "ON": true, " true ": true, "1": true, "yes": true,
	}
	for value, want := range tests {
		if got := Enabled(value); got != want {
			t.Errorf("Enabled(%q) = %v, want %v", value, got, want)
		}
	}
}

// fakePulse records pactl/pulseaudio calls and answers from its fields.
type fakePulse struct {
	running bool
	modules string
	fail    string
	calls   []string
}

func (f *fakePulse) run(_ context.Context, name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.fail != "" && strings.Contains(call, f.fail) {
		return "boom", errors.New("exit status 1")
	}
	switch {
	case call == "pulseaudio --check":
		if !f.running {
			return "", errors.New("exit status 1")
		}
	case strings.HasPrefix(call, "pactl list short modules"):
		return f.modules, nil
	}
	return "", nil
}

func (f *fakePulse) called(fragment string) bool {
	for _, call := range f.calls {
		if strings.Contains(call, fragment) {
			return true
		}
	}
	return false
}

func newTestMic(t *testing.T, pulse *fakePulse) *Mic {
	t.Helper()
	mic := New()
	mic.run = pulse.run
	mic.asoundrc = filepath.Join(t.TempDir(), ".asoundrc")
	return mic
}

func TestSetup(t *testing.T) {
	t.Run("fresh host starts PulseAudio and creates the microphone", func(t *testing.T) {
		pulse := &fakePulse{modules: "12\tmodule-suspend-on-idle\t\t"}
		mic := newTestMic(t, pulse)
		if err := mic.Setup(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"pulseaudio --start",
			"load-module module-null-sink sink_name=atlas_mic rate=16000 channels=1 format=s16le",
			"unload-module module-suspend-on-idle",
			"set-default-source atlas_mic.monitor",
		} {
			if !pulse.called(want) {
				t.Errorf("missing call %q in %v", want, pulse.calls)
			}
		}
		if status := mic.Status(); !status.Enabled || !status.Ready || status.Error != "" {
			t.Errorf("status = %+v", status)
		}
		written, err := os.ReadFile(mic.asoundrc)
		if err != nil || !routesToPulse(string(written)) {
			t.Errorf("asoundrc = %q, err %v", written, err)
		}
	})

	t.Run("repeat setup does not duplicate the microphone", func(t *testing.T) {
		pulse := &fakePulse{running: true, modules: "19\tmodule-null-sink\tsink_name=atlas_mic rate=16000"}
		mic := newTestMic(t, pulse)
		if err := mic.Setup(context.Background()); err != nil {
			t.Fatal(err)
		}
		if pulse.called("pulseaudio --start") || pulse.called("load-module") || pulse.called("unload-module") {
			t.Errorf("unexpected calls %v", pulse.calls)
		}
	})

	t.Run("a foreign asoundrc is never replaced", func(t *testing.T) {
		pulse := &fakePulse{running: true}
		mic := newTestMic(t, pulse)
		foreign := []byte("pcm.!default { type hw; card 1 }\n")
		if err := os.WriteFile(mic.asoundrc, foreign, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := mic.Setup(context.Background()); err == nil {
			t.Fatal("expected an error")
		}
		kept, _ := os.ReadFile(mic.asoundrc)
		if !bytes.Equal(kept, foreign) {
			t.Errorf("asoundrc changed to %q", kept)
		}
		if status := mic.Status(); status.Ready || !strings.Contains(status.Error, "type pulse") {
			t.Errorf("status = %+v", status)
		}
		if pulse.called("set-default-source") {
			t.Error("must not change the default source after failing")
		}
	})

	t.Run("an equivalent asoundrc with other spacing is accepted", func(t *testing.T) {
		mic := newTestMic(t, &fakePulse{running: true})
		if err := os.WriteFile(mic.asoundrc, []byte("pcm.!default  {\n  type pulse\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := mic.Setup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a failed command reports its output", func(t *testing.T) {
		mic := newTestMic(t, &fakePulse{running: true, fail: "load-module"})
		err := mic.Setup(context.Background())
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v", err)
		}
	})
}

// fakeSink records written frames.
type fakeSink struct {
	mu     sync.Mutex
	data   bytes.Buffer
	closed chan struct{}
	once   sync.Once
}

func newFakeSink() *fakeSink { return &fakeSink{closed: make(chan struct{})} }

func (s *fakeSink) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Write(b)
}

func (s *fakeSink) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeSink) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.data.Bytes()...)
}

func waitClosed(t *testing.T, s *fakeSink) {
	t.Helper()
	select {
	case <-s.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("sink was not closed")
	}
}

func startStreamServer(t *testing.T, sinks ...*fakeSink) *httptest.Server {
	t.Helper()
	mic := newTestMic(t, &fakePulse{running: true})
	next := 0
	var mu sync.Mutex
	mic.start = func() (playback, error) {
		mu.Lock()
		defer mu.Unlock()
		sink := sinks[next]
		next++
		return sink, nil
	}
	server := httptest.NewServer(http.HandlerFunc(mic.HandleStream))
	t.Cleanup(server.Close)
	return server
}

func dial(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestHandleStream(t *testing.T) {
	t.Run("forwards PCM and stops playback on disconnect", func(t *testing.T) {
		sink := newFakeSink()
		conn := dial(t, startStreamServer(t, sink))
		frame := []byte{1, 2, 3, 4}
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		waitClosed(t, sink)
		if got := sink.bytes(); !bytes.Equal(got, frame) {
			t.Errorf("sink = %v, want %v", got, frame)
		}
	})

	t.Run("rejects frames that are not 16-bit PCM", func(t *testing.T) {
		for name, send := range map[string]func(*websocket.Conn) error{
			"text":     func(c *websocket.Conn) error { return c.WriteMessage(websocket.TextMessage, []byte("hi")) },
			"odd size": func(c *websocket.Conn) error { return c.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3}) },
		} {
			t.Run(name, func(t *testing.T) {
				sink := newFakeSink()
				conn := dial(t, startStreamServer(t, sink))
				if err := send(conn); err != nil {
					t.Fatal(err)
				}
				waitClosed(t, sink)
				if got := sink.bytes(); len(got) != 0 {
					t.Errorf("sink received %v", got)
				}
			})
		}
	})

	t.Run("rejects oversized frames", func(t *testing.T) {
		sink := newFakeSink()
		conn := dial(t, startStreamServer(t, sink))
		if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, maxFrameBytes+2)); err != nil {
			t.Fatal(err)
		}
		waitClosed(t, sink)
		if got := sink.bytes(); len(got) != 0 {
			t.Errorf("sink received %d bytes", len(got))
		}
	})

	t.Run("the newest stream takes over", func(t *testing.T) {
		first, second := newFakeSink(), newFakeSink()
		server := startStreamServer(t, first, second)
		firstConn := dial(t, server)
		if err := firstConn.WriteMessage(websocket.BinaryMessage, []byte{1, 1}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for len(first.bytes()) == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		secondConn := dial(t, server)
		waitClosed(t, first)
		if err := secondConn.WriteMessage(websocket.BinaryMessage, []byte{2, 2}); err != nil {
			t.Fatal(err)
		}
		deadline = time.Now().Add(5 * time.Second)
		for len(second.bytes()) == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if got := second.bytes(); !bytes.Equal(got, []byte{2, 2}) {
			t.Errorf("second sink = %v", got)
		}
		select {
		case <-second.closed:
			t.Error("second stream must stay open")
		default:
		}
	})
}

func TestHandleStreamReportsSetupFailure(t *testing.T) {
	mic := newTestMic(t, &fakePulse{running: true, fail: "set-default-source"})
	mic.start = func() (playback, error) {
		t.Error("playback must not start when setup failed")
		return nil, errors.New("unexpected")
	}
	recorder := httptest.NewRecorder()
	mic.HandleStream(recorder, httptest.NewRequest(http.MethodGet, "/ws/mic", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d", recorder.Code)
	}
}
