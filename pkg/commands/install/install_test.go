package install

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

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

func TestAgentServiceTemplatesAreHeadless(t *testing.T) {
	config := serviceConfig{
		BinaryPath: "/home/user/.local/bin/tmuxatlas",
		ExecStart:  "/home/user/.local/bin/tmuxatlas agent",
		Path:       "/usr/bin", LogDir: "/tmp",
		Description: "TmuxAtlas agent", Command: "agent",
		Label: "com.tmuxatlas.agent", LogName: "com.tmuxatlas.agent",
		EnvironmentName: "TMUXATLAS_HUB", EnvironmentValue: "https://hub.example.com",
		Role: "agent",
	}
	for name, source := range map[string]string{"systemd": systemdUnit, "launchd": launchdPlist} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			tmpl, err := template.New(name).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := tmpl.Execute(&output, config); err != nil {
				t.Fatal(err)
			}
			rendered := output.String()
			for _, required := range []string{"agent", "TMUXATLAS_HUB", "https://hub.example.com", "TMUXATLAS_ROLE"} {
				if !strings.Contains(rendered, required) {
					t.Fatalf("rendered service does not contain %q:\n%s", required, rendered)
				}
			}
			for _, forbidden := range []string{"TMUXATLAS_PUBLIC_URL", "passkey", "127.0.0.1:7654"} {
				if strings.Contains(rendered, forbidden) {
					t.Fatalf("headless service contains %q:\n%s", forbidden, rendered)
				}
			}
		})
	}
}

func TestHubServiceTemplatesUsePureHubCommand(t *testing.T) {
	config := serviceConfig{
		BinaryPath: "/home/user/.local/bin/tmuxatlas",
		ExecStart:  "/home/user/.local/bin/tmuxatlas hub",
		Path:       "/usr/bin", LogDir: "/tmp", Description: "TmuxAtlas Hub",
		Command: "hub", Label: "com.tmuxatlas.server", LogName: "com.tmuxatlas.server",
		EnvironmentName: "TMUXATLAS_PUBLIC_URL", EnvironmentValue: "https://hub.example.com",
		Role: "hub",
	}
	for name, source := range map[string]string{"systemd": systemdUnit, "launchd": launchdPlist} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			tmpl, err := template.New(name).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := tmpl.Execute(&output, config); err != nil {
				t.Fatal(err)
			}
			rendered := output.String()
			requiredValues := []string{"TMUXATLAS_ROLE", "hub"}
			if name == "systemd" {
				requiredValues = append(requiredValues, "tmuxatlas hub")
			} else {
				requiredValues = append(requiredValues, "<string>/home/user/.local/bin/tmuxatlas</string>", "<string>hub</string>")
			}
			for _, required := range requiredValues {
				if !strings.Contains(rendered, required) {
					t.Fatalf("rendered service does not contain %q:\n%s", required, rendered)
				}
			}
		})
	}
}
