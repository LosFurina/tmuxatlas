package tmux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPTYCloseForcesUnresponsiveAttachProcessAndIsIdempotent(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-tmux")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap '' HUP INT TERM\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousClose, previousTerminate := ptyCloseGrace, ptyTerminateGrace
	ptyCloseGrace, ptyTerminateGrace = 20*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() {
		ptyCloseGrace, ptyTerminateGrace = previousClose, previousTerminate
	})
	session, err := NewPTYSession(script, "work", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	session.Close()
	session.Close()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("PTY Close took %s", elapsed)
	}
	if session.cmd.ProcessState == nil {
		t.Fatal("PTY attach subprocess was not reaped")
	}
}
