package server

import (
	"strings"
	"testing"
)

func TestValidateListenAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7654", "localhost:7654", "[::1]:7654"} {
		if err := validateListenAddress(address); err != nil {
			t.Errorf("validateListenAddress(%q): %v", address, err)
		}
	}
	for _, address := range []string{"", ":7654", "127.0.0.1", "localhost:"} {
		if err := validateListenAddress(address); err == nil {
			t.Errorf("validateListenAddress(%q) unexpectedly succeeded", address)
		}
	}
}

func TestValidatePublicURL(t *testing.T) {
	tests := []struct {
		raw    string
		scheme string
		ok     bool
	}{
		{raw: "http://localhost:7654", scheme: "http", ok: true},
		{raw: "https://tmuxatlas.example.com", scheme: "https", ok: true},
		{raw: "tmuxatlas.example.com", ok: false},
		{raw: "ftp://tmuxatlas.example.com", ok: false},
		{raw: "https://tmuxatlas.example.com/prefix", ok: false},
	}
	for _, tt := range tests {
		u, err := validatePublicURL(tt.raw)
		if tt.ok && err != nil {
			t.Errorf("validatePublicURL(%q): %v", tt.raw, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("validatePublicURL(%q) unexpectedly succeeded", tt.raw)
		}
		if tt.ok && u.Scheme != tt.scheme {
			t.Errorf("validatePublicURL(%q) scheme = %q, want %q", tt.raw, u.Scheme, tt.scheme)
		}
	}
}

func TestServerFlagsUseHTTPOriginConfiguration(t *testing.T) {
	names := map[string]bool{}
	for _, flag := range serverFlags() {
		for _, name := range flag.Names() {
			names[name] = true
		}
	}
	for _, required := range []string{"listen", "public-url", "session-ttl"} {
		if !names[required] {
			t.Errorf("missing %q flag", required)
		}
	}
	for _, removed := range []string{"port", "no-tls", "tls-cert", "tls-key", "tls-san", "tls-reload-interval", "insecure"} {
		if names[removed] {
			t.Errorf("obsolete flag %q is still registered", removed)
		}
	}
	if defaultListenAddress != "127.0.0.1:7654" {
		t.Fatalf("default listen address = %q", defaultListenAddress)
	}
}

func TestHubFlagsExcludeTmuxIntegration(t *testing.T) {
	names := map[string]bool{}
	for _, flag := range hubFlags() {
		for _, name := range flag.Names() {
			names[name] = true
		}
	}
	for _, required := range []string{"listen", "public-url", "session-ttl", "socket"} {
		if !names[required] {
			t.Errorf("missing Hub flag %q", required)
		}
	}
	for _, local := range []string{"discovery-interval", "no-control-mode", "hub", "local-only"} {
		if names[local] {
			t.Errorf("pure Hub exposed local integration flag %q", local)
		}
	}
}

func TestRemovedTransportEnvironmentRejected(t *testing.T) {
	t.Setenv("TMUXATLAS_TLS_CERT", "/tmp/cert.pem")
	err := validateRemovedTransportEnv()
	if err == nil || !strings.Contains(err.Error(), "TMUXATLAS_TLS_CERT is no longer supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLegacyRemovedTransportEnvironmentRejected(t *testing.T) {
	t.Setenv("GUPPI_TLS_CERT", "/tmp/cert.pem")
	err := validateRemovedTransportEnv()
	if err == nil || !strings.Contains(err.Error(), "GUPPI_TLS_CERT is no longer supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNoAuthModeDefersIngressChoiceToOperator(t *testing.T) {
	local, _ := validatePublicURL("http://localhost:7654")
	external, _ := validatePublicURL("https://tmuxatlas.example")
	if err := validateNoAuthMode(true, "127.0.0.1:7654", local); err != nil {
		t.Fatalf("local no-auth rejected: %v", err)
	}
	if err := validateNoAuthMode(true, "0.0.0.0:7654", local); err != nil {
		t.Fatalf("wildcard no-auth rejected: %v", err)
	}
	if err := validateNoAuthMode(true, "0.0.0.0:7654", external); err != nil {
		t.Fatalf("external no-auth origin rejected: %v", err)
	}
}
