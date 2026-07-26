package peer

import (
	"testing"
)

func TestHubWebSocketURL(t *testing.T) {
	for _, tt := range []struct {
		input string
		path  string
		want  string
	}{
		{input: "hub.example.com", path: "/ws/peer", want: "wss://hub.example.com/ws/peer"},
		{input: "https://hub.example.com", path: "/ws/peer", want: "wss://hub.example.com/ws/peer"},
		{input: "http://127.0.0.1:7654", path: "/ws/peer-pty", want: "ws://127.0.0.1:7654/ws/peer-pty"},
		{input: "ws://localhost:7654", path: "/ws/peer", want: "ws://localhost:7654/ws/peer"},
	} {
		u, err := hubWebSocketURL(tt.input, tt.path)
		if err != nil {
			t.Fatalf("hubWebSocketURL(%q): %v", tt.input, err)
		}
		if u.String() != tt.want {
			t.Errorf("hubWebSocketURL(%q) = %q, want %q", tt.input, u, tt.want)
		}
	}
	if _, err := hubWebSocketURL("ftp://hub.example.com", "/ws/peer"); err == nil {
		t.Fatal("unsupported scheme unexpectedly accepted")
	}
	if _, err := hubWebSocketURL("https://hub.example.com/prefix", "/ws/peer"); err == nil {
		t.Fatal("path-prefixed hub URL unexpectedly accepted")
	}
}
