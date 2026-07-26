package ws

import (
	"net/http"
	"testing"
)

func TestCheckSameOriginBehindHostPreservingProxy(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{
			name:   "public host preserved",
			host:   "tmuxatlas.example.com",
			origin: "https://tmuxatlas.example.com",
			want:   true,
		},
		{
			name:   "public host with port preserved",
			host:   "tmuxatlas.example.com:8443",
			origin: "https://tmuxatlas.example.com:8443",
			want:   true,
		},
		{
			name:   "rewritten host rejected",
			host:   "127.0.0.1:7654",
			origin: "https://tmuxatlas.example.com",
			want:   false,
		},
		{
			name:   "cross-site origin rejected",
			host:   "tmuxatlas.example.com",
			origin: "https://attacker.example",
			want:   false,
		},
		{
			name: "non-browser client accepted",
			host: "tmuxatlas.example.com",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Host: tt.host, Header: make(http.Header)}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := CheckSameOrigin(req); got != tt.want {
				t.Fatalf("CheckSameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
