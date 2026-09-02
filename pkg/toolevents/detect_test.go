package toolevents

import "testing"

func TestAgentPatternsRecognizeAgyAndOpenCodeNodeWrappers(t *testing.T) {
	cases := []struct {
		args  []string
		want  Tool
		found bool
	}{
		{[]string{"agy"}, ToolAgy, true},
		{[]string{"/home/me/.local/bin/agy"}, ToolAgy, true},
		{[]string{"node", "/home/me/.npm/opencode/bin/cli.js"}, ToolOpenCode, true},
		{[]string{"node", "/tmp/not-opencode-related.js"}, "", false},
		{[]string{"agy-helper"}, "", false},
	}
	for _, tc := range cases {
		got, ok := detectFromArgs(tc.args)
		if got != tc.want || ok != tc.found {
			t.Fatalf("%v: got %q,%v want %q,%v", tc.args, got, ok, tc.want, tc.found)
		}
	}
}
