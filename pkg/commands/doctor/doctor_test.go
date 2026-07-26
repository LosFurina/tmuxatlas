package doctor

import "testing"

func TestPublicURLCheck(t *testing.T) {
	for _, test := range []struct {
		value string
		level level
	}{
		{value: "https://tmuxatlas.example.com", level: levelOK},
		{value: "http://localhost:7654", level: levelOK},
		{value: "http://127.0.0.1:7654", level: levelOK},
		{value: "http://tmuxatlas.example.com", level: levelFail},
		{value: "https://tmuxatlas.example.com/path", level: levelFail},
	} {
		if got := publicURLCheck(test.value).Level; got != test.level {
			t.Errorf("publicURLCheck(%q) = %s, want %s", test.value, got, test.level)
		}
	}
}

func TestSessionTTLCheck(t *testing.T) {
	for _, test := range []struct {
		value string
		level level
	}{
		{value: "24h", level: levelOK},
		{value: "168h", level: levelOK},
		{value: "1m", level: levelOK},
		{value: "59s", level: levelFail},
		{value: "forever", level: levelFail},
	} {
		if got := sessionTTLCheck(test.value).Level; got != test.level {
			t.Errorf("sessionTTLCheck(%q) = %s, want %s", test.value, got, test.level)
		}
	}
}
