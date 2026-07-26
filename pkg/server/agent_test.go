package server

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/toolevents"
)

func TestRunAgentSocketAcceptsLocalToolEvents(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "tmuxatlas-agent-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := filepath.Join(tempDir, "agent.sock")
	tracker := toolevents.NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- RunAgentSocket(ctx, socketPath, tracker, nil) }()

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}}
	var response *http.Response
	err = nil
	for attempt := 0; attempt < 50; attempt++ {
		response, err = client.Post("http://localhost/api/tool-event", "application/json",
			bytes.NewBufferString(`{"tool":"codex","status":"waiting","session":"test"}`))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.StatusCode)
	}
	if events := tracker.GetForSession("test"); len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
