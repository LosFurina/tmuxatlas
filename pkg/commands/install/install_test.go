package install

import "testing"

func TestValidatePublicURL(t *testing.T) {
	for _, test := range []struct {
		value string
		ok    bool
	}{
		{value: "https://tmuxatlas.example.com", ok: true},
		{value: "https://tmuxatlas.example.com:8443", ok: true},
		{value: "http://localhost:7654", ok: true},
		{value: "http://127.0.0.1:7654", ok: true},
		{value: "http://[::1]:7654", ok: true},
		{value: "http://tmuxatlas.example.com", ok: false},
		{value: "https://tmuxatlas.example.com/path", ok: false},
		{value: "not-a-url", ok: false},
	} {
		t.Run(test.value, func(t *testing.T) {
			_, err := validatePublicURL(test.value)
			if (err == nil) != test.ok {
				t.Fatalf("validatePublicURL(%q) error = %v, want ok=%v", test.value, err, test.ok)
			}
		})
	}
}
