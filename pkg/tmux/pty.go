package tmux

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty/v2"
	"github.com/sirupsen/logrus"
)

var (
	ptyCloseGrace     = 2 * time.Second
	ptyTerminateGrace = time.Second
)

// PTYSession wraps a PTY running `tmux attach-session`
type PTYSession struct {
	cmd   *exec.Cmd
	ptyFd *os.File
	close sync.Once
}

// NewPTYSession spawns `tmux attach-session -t <session>` in a PTY
func NewPTYSession(tmuxPath, sessionName string, cols, rows uint16) (*PTYSession, error) {
	cmd := exec.Command(tmuxPath, "attach-session", "-t", sessionName)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"session": sessionName,
		"cols":    cols,
		"rows":    rows,
	}).Info("started PTY session")

	return &PTYSession{
		cmd:   cmd,
		ptyFd: f,
	}, nil
}

// Read reads from the PTY master fd
func (p *PTYSession) Read(buf []byte) (int, error) {
	return p.ptyFd.Read(buf)
}

// Write writes to the PTY master fd
func (p *PTYSession) Write(data []byte) (int, error) {
	return p.ptyFd.Write(data)
}

// Resize changes the PTY window size
func (p *PTYSession) Resize(cols, rows uint16) error {
	return pty.Setsize(p.ptyFd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Close closes the PTY and waits for the subprocess to exit
func (p *PTYSession) Close() {
	p.close.Do(func() {
		_ = p.ptyFd.Close()
		done := make(chan struct{})
		go func() {
			_ = p.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(ptyCloseGrace):
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Signal(os.Interrupt)
			}
			select {
			case <-done:
			case <-time.After(ptyTerminateGrace):
				if p.cmd.Process != nil {
					_ = p.cmd.Process.Kill()
				}
				<-done
			}
		}
		logrus.Debug("PTY session closed")
	})
}
