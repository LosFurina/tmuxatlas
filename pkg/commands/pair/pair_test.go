package pair

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "hub.example.com", want: "https://hub.example.com"},
		{input: "http://127.0.0.1:7654", want: "http://127.0.0.1:7654"},
		{input: "https://hub.example.com", want: "https://hub.example.com"},
	} {
		u, err := normalizeURL(tt.input)
		if err != nil {
			t.Fatal(err)
		}
		if u.String() != tt.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, u, tt.want)
		}
	}
	for _, input := range []string{
		"ftp://hub.example.com",
		"https://hub.example.com/prefix",
		"https://user@hub.example.com",
	} {
		if _, err := normalizeURL(input); err == nil {
			t.Errorf("normalizeURL(%q) unexpectedly succeeded", input)
		}
	}
}

func TestPairHTTPClientRejectsUntrustedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if _, err := newPairHTTPClient().Get(server.URL); err == nil {
		t.Fatal("pair client unexpectedly accepted an untrusted certificate")
	}
}
