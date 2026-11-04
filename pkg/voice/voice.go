// Package voice bridges a browser microphone to a PulseAudio virtual microphone
// so that terminal programs on this host that capture from the default audio
// device (for example Claude Code /voice through SoX) hear the browser user.
//
//	browser mic -> /ws/mic -> pacat -> null sink "atlas_mic" -> monitor source -> ALSA pulse plugin -> SoX
//
// A null sink is used rather than a pipe source because PulseAudio clocks it: an
// open recorder receives real-time silence when nobody is streaming, and audio
// never queues up to be replayed later.
package voice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/LosFurina/tmuxatlas/pkg/ws"
)

const (
	sinkName   = "atlas_mic"
	sourceName = sinkName + ".monitor"

	// SampleRate, 16-bit little-endian mono, is the format the browser must send.
	SampleRate = 16000

	// maxFrameBytes bounds one WebSocket message. Browsers send 20 ms (640 bytes).
	maxFrameBytes = 16 << 10
	// frameTimeout closes a stream whose browser stopped sending. A live
	// microphone sends 50 frames per second, even when the room is silent.
	frameTimeout = 15 * time.Second

	commandTimeout = 5 * time.Second
)

// asoundrc routes ALSA clients such as SoX to PulseAudio.
const asoundrc = `# Managed by TmuxAtlas (TMUXATLAS_VOICE): ALSA capture goes through PulseAudio.
pcm.!default { type pulse }
ctl.!default { type pulse }
`

// Status reports whether the browser microphone can be used on this host.
type Status struct {
	Enabled bool   `json:"enabled"`
	Ready   bool   `json:"ready"`
	Error   string `json:"error,omitempty"`
}

// Enabled reads TMUXATLAS_VOICE. Voice stays off unless explicitly enabled
// because setup changes the default audio source of the user running TmuxAtlas.
func Enabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes":
		return true
	default:
		return false
	}
}

// runner executes a command and returns its combined output.
type runner func(ctx context.Context, name string, args ...string) (string, error)

func execRunner(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// playback is the running audio sink for one browser stream.
type playback interface {
	io.Writer
	Close() error
}

type pacatPlayback struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func startPacat() (playback, error) {
	cmd := exec.Command("pacat",
		"--playback", "--raw",
		"--format=s16le", fmt.Sprintf("--rate=%d", SampleRate), "--channels=1",
		"--latency-msec=30",
		"--device="+sinkName,
		"--stream-name=tmuxatlas-browser-mic",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &pacatPlayback{cmd: cmd, stdin: stdin}, nil
}

func (p *pacatPlayback) Write(b []byte) (int, error) { return p.stdin.Write(b) }

func (p *pacatPlayback) Close() error {
	p.stdin.Close()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	return p.cmd.Wait()
}

// Mic owns the virtual microphone and the single active browser stream.
type Mic struct {
	run      runner
	start    func() (playback, error)
	asoundrc string
	upgrader websocket.Upgrader

	mu         sync.Mutex
	status     Status
	generation uint64
	cancelCur  func()
}

// New creates a Mic for the current user. Call Setup before serving streams.
func New() *Mic {
	home, _ := os.UserHomeDir()
	return &Mic{
		run:      execRunner,
		start:    startPacat,
		asoundrc: filepath.Join(home, ".asoundrc"),
		upgrader: websocket.Upgrader{
			CheckOrigin:     ws.CheckSameOrigin,
			ReadBufferSize:  4 << 10,
			WriteBufferSize: 1 << 10,
		},
		status: Status{Enabled: true},
	}
}

// Status returns the result of the latest Setup.
func (m *Mic) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Setup makes the virtual microphone the default capture device. It is
// idempotent and runs again for each stream, so a restarted PulseAudio recovers.
func (m *Mic) Setup(ctx context.Context) error {
	err := m.setup(ctx)
	m.mu.Lock()
	m.status = Status{Enabled: true, Ready: err == nil}
	if err != nil {
		m.status.Error = err.Error()
	}
	m.mu.Unlock()
	return err
}

func (m *Mic) setup(ctx context.Context) error {
	if err := m.ensureAsoundrc(); err != nil {
		return err
	}
	if _, err := m.run(ctx, "pulseaudio", "--check"); err != nil {
		if out, startErr := m.run(ctx, "pulseaudio", "--start", "--exit-idle-time=-1"); startErr != nil {
			return commandError("start PulseAudio", out, startErr)
		}
	}
	modules, err := m.run(ctx, "pactl", "list", "short", "modules")
	if err != nil {
		return commandError("list PulseAudio modules", modules, err)
	}
	if !strings.Contains(modules, "sink_name="+sinkName) {
		out, err := m.run(ctx, "pactl", "load-module", "module-null-sink",
			"sink_name="+sinkName,
			fmt.Sprintf("rate=%d", SampleRate), "channels=1", "format=s16le",
			"sink_properties=device.description=TmuxAtlas_Mic",
		)
		if err != nil {
			return commandError("create the virtual microphone", out, err)
		}
	}
	// A suspended sink adds a resume delay that would clip the start of dictation.
	if strings.Contains(modules, "module-suspend-on-idle") {
		if out, err := m.run(ctx, "pactl", "unload-module", "module-suspend-on-idle"); err != nil {
			return commandError("disable PulseAudio idle suspend", out, err)
		}
	}
	if out, err := m.run(ctx, "pactl", "set-default-source", sourceName); err != nil {
		return commandError("select the virtual microphone", out, err)
	}
	return nil
}

// ensureAsoundrc writes the ALSA routing file, refusing to replace a file that
// does not already route the default device to PulseAudio.
func (m *Mic) ensureAsoundrc() error {
	existing, err := os.ReadFile(m.asoundrc)
	switch {
	case err == nil:
		if routesToPulse(string(existing)) {
			return nil
		}
		return fmt.Errorf("%s exists and does not route the default ALSA device to PulseAudio; add: pcm.!default { type pulse }", m.asoundrc)
	case errors.Is(err, os.ErrNotExist):
		if err := os.WriteFile(m.asoundrc, []byte(asoundrc), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", m.asoundrc, err)
		}
		return nil
	default:
		return fmt.Errorf("read %s: %w", m.asoundrc, err)
	}
}

func routesToPulse(content string) bool {
	return strings.Contains(strings.Join(strings.Fields(content), " "), "pcm.!default { type pulse }")
}

func commandError(action, output string, err error) error {
	if output != "" {
		return fmt.Errorf("%s: %s", action, output)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// claim makes the caller the active stream. The newest stream wins so that two
// tabs never mix their microphones; the previous one is disconnected.
func (m *Mic) claim(cancel func()) (release func()) {
	m.mu.Lock()
	previous := m.cancelCur
	m.generation++
	generation := m.generation
	m.cancelCur = cancel
	m.mu.Unlock()
	if previous != nil {
		previous()
	}
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.generation == generation {
			m.cancelCur = nil
		}
	}
}

// HandleStream receives PCM frames from one browser and plays them into the
// virtual microphone. Audio is forwarded only and never stored.
func (m *Mic) HandleStream(w http.ResponseWriter, r *http.Request) {
	if err := m.Setup(r.Context()); err != nil {
		logrus.WithError(err).Warn("voice setup failed")
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.WithError(err).Warn("mic ws upgrade failed")
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxFrameBytes)

	sink, err := m.start()
	if err != nil {
		logrus.WithError(err).Error("failed to start mic playback")
		closeWith(conn, websocket.CloseInternalServerErr, "microphone playback failed")
		return
	}
	var closeSink sync.Once
	stopSink := func() { closeSink.Do(func() { sink.Close() }) }
	defer stopSink()

	var takenOver sync.Once
	release := m.claim(func() {
		takenOver.Do(func() {
			closeWith(conn, websocket.ClosePolicyViolation, "microphone taken over by another tab")
			stopSink()
			conn.Close()
		})
	})
	defer release()

	logrus.Info("browser microphone connected")
	defer logrus.Info("browser microphone disconnected")
	for {
		conn.SetReadDeadline(time.Now().Add(frameTimeout))
		messageType, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.BinaryMessage || len(frame)%2 != 0 {
			closeWith(conn, websocket.CloseUnsupportedData, "expected 16-bit PCM frames")
			return
		}
		if _, err := sink.Write(frame); err != nil {
			logrus.WithError(err).Warn("mic playback stopped")
			closeWith(conn, websocket.CloseInternalServerErr, "microphone playback stopped")
			return
		}
	}
}

func closeWith(conn *websocket.Conn, code int, reason string) {
	conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
}
