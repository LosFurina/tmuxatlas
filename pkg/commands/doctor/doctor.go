package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

type level string

const (
	levelOK   level = "OK"
	levelWarn level = "WARN"
	levelFail level = "FAIL"
)

type check struct {
	Level   level
	Name    string
	Message string
}

func publicURLCheck(raw string) check {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.Fragment != "" {
		return check{levelFail, "public URL", "TMUXATLAS_PUBLIC_URL must be an absolute HTTP(S) origin without a path"}
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return check{levelFail, "public URL", "remote Passkeys require HTTPS"}
		}
	}
	return check{levelOK, "public URL", raw}
}

func sessionTTLCheck(raw string) check {
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl < time.Minute {
		return check{levelFail, "session TTL", "TMUXATLAS_SESSION_TTL must be a duration of at least 1m"}
	}
	return check{levelOK, "session TTL", ttl.String() + " sliding idle timeout"}
}

func fileModeCheck(name, path string, required os.FileMode) check {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return check{levelWarn, name, path + " does not exist"}
	}
	if err != nil {
		return check{levelFail, name, err.Error()}
	}
	if info.Mode().Perm() != required {
		return check{levelFail, name, fmt.Sprintf("%s mode is %04o, want %04o", path, info.Mode().Perm(), required)}
	}
	return check{levelOK, name, path}
}

func runChecks() []check {
	var checks []check
	checks = append(checks, check{levelOK, "version", common.SUMMARY + " (" + common.COMMIT + ")"})

	if executable, err := os.Executable(); err != nil {
		checks = append(checks, check{levelFail, "executable", err.Error()})
	} else if resolved, err := filepath.EvalSymlinks(executable); err != nil {
		checks = append(checks, check{levelFail, "executable", err.Error()})
	} else {
		checks = append(checks, check{levelOK, "executable", resolved})
	}

	if tmuxPath, err := exec.LookPath("tmux"); err != nil {
		checks = append(checks, check{levelFail, "tmux", "tmux was not found in PATH"})
	} else {
		output, err := exec.Command(tmuxPath, "-V").CombinedOutput()
		if err != nil {
			checks = append(checks, check{levelFail, "tmux", strings.TrimSpace(string(output))})
		} else {
			checks = append(checks, check{levelOK, "tmux", strings.TrimSpace(string(output))})
		}
	}

	configDir, err := paths.ConfigDir()
	if err != nil {
		checks = append(checks, check{levelFail, "config directory", err.Error()})
		return checks
	}
	checks = append(checks, check{levelOK, "config directory", configDir})
	checks = append(checks, fileModeCheck("environment file", filepath.Join(configDir, ".env"), 0o600))

	publicURL := firstNonEmpty(os.Getenv("TMUXATLAS_PUBLIC_URL"), "http://localhost:7654")
	checks = append(checks, publicURLCheck(publicURL))
	sessionTTL := firstNonEmpty(os.Getenv("TMUXATLAS_SESSION_TTL"), "24h")
	checks = append(checks, sessionTTLCheck(sessionTTL))

	passkeyPath := filepath.Join(configDir, "passkeys.json")
	passkeyCheck := fileModeCheck("Passkey store", passkeyPath, 0o600)
	if passkeyCheck.Level == levelOK {
		raw, readErr := os.ReadFile(passkeyPath)
		var stored struct {
			RPID        string            `json:"rp_id"`
			Origin      string            `json:"origin"`
			Credentials []json.RawMessage `json:"credentials"`
		}
		if readErr != nil || json.Unmarshal(raw, &stored) != nil {
			passkeyCheck = check{levelFail, "Passkey store", "passkeys.json is not valid JSON"}
		} else if len(stored.Credentials) == 0 {
			passkeyCheck = check{levelWarn, "Passkey store", "no Passkey has been enrolled yet"}
		} else {
			passkeyCheck.Message = fmt.Sprintf("%d credential(s), RP ID %s", len(stored.Credentials), stored.RPID)
		}
	}
	checks = append(checks, passkeyCheck)
	if _, err := os.Stat(filepath.Join(configDir, "auth.json")); err == nil {
		checks = append(checks, check{levelWarn, "legacy password", "auth.json is ignored and can be removed after Passkey login is verified"})
	}

	listen := firstNonEmpty(os.Getenv("TMUXATLAS_LISTEN"), "127.0.0.1:7654")
	connection, err := net.DialTimeout("tcp", listen, 300*time.Millisecond)
	if err != nil {
		checks = append(checks, check{levelWarn, "server", "not listening on " + listen})
	} else {
		connection.Close()
		checks = append(checks, check{levelOK, "server", "listening on " + listen})
	}

	servicePath := ""
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		servicePath = filepath.Join(home, "Library", "LaunchAgents", "com.tmuxatlas.server.plist")
	case "linux":
		if configHome, err := os.UserConfigDir(); err == nil {
			servicePath = filepath.Join(configHome, "systemd", "user", "tmuxatlas.service")
		}
	}
	if servicePath != "" {
		if _, err := os.Stat(servicePath); err == nil {
			checks = append(checks, check{levelOK, "user service", servicePath})
		} else {
			checks = append(checks, check{levelWarn, "user service", "not installed; run tmuxatlas install"})
		}
	}
	return checks
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func execute(_ context.Context, _ *cli.Command) error {
	checks := runChecks()
	failures, warnings := 0, 0
	for _, result := range checks {
		fmt.Printf("[%-4s] %-18s %s\n", result.Level, result.Name, result.Message)
		switch result.Level {
		case levelFail:
			failures++
		case levelWarn:
			warnings++
		}
	}
	fmt.Printf("\n%d checks, %d warning(s), %d failure(s)\n", len(checks), warnings, failures)
	if failures > 0 {
		return fmt.Errorf("doctor found %d failure(s)", failures)
	}
	return nil
}

func init() {
	common.RegisterCommand(&cli.Command{
		Name:   "doctor",
		Usage:  "diagnose the local TmuxAtlas installation",
		Action: execute,
	})
}
