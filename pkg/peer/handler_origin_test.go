package peer

import (
	"net/http/httptest"
	"testing"
)

func TestPeerUpgraderRejectsBrowserOrigins(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "peer client without origin", origin: "", want: true},
		{name: "cross-site browser", origin: "https://evil.example", want: false},
		{name: "same-origin browser", origin: "http://example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://example.com/ws/peer", nil)
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			if got := wsUpgrader.CheckOrigin(request); got != tt.want {
				t.Fatalf("CheckOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
