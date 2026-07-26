package install

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/urfave/cli/v3"

	"github.com/LosFurina/tmuxatlas/pkg/common"
)

const systemdUnit = `[Unit]
Description=TmuxAtlas - Web dashboard for tmux sessions
After=default.target

[Service]
Type=simple
ExecStart={{.ExecStart}}
Restart=on-failure
RestartSec=5
Environment=PATH={{.Path}}
Environment=TMUXATLAS_PUBLIC_URL={{.PublicURL}}

[Install]
WantedBy=default.target
`

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.tmuxatlas.server</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		<string>server</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/tmuxatlas.stdout.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/tmuxatlas.stderr.log</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>{{.Path}}</string>
		<key>TMUXATLAS_PUBLIC_URL</key>
		<string>{{.PublicURL}}</string>
	</dict>
</dict>
</plist>
`

type serviceConfig struct {
	BinaryPath string
	ExecStart  string
	Path       string
	LogDir     string
	PublicURL  string
}

func validatePublicURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("public URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("public URL must use HTTP or HTTPS")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("public URL must not contain credentials, a path, query, or fragment")
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return "", fmt.Errorf("public URL must use HTTPS except on localhost")
		}
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func getBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not determine executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("could not resolve symlinks: %w", err)
	}
	return exe, nil
}

func installLinux(ctx context.Context, c *cli.Command) error {
	publicURL, err := validatePublicURL(c.String("public-url"))
	if err != nil {
		return err
	}
	binPath, err := getBinaryPath()
	if err != nil {
		return err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("could not determine config directory: %w", err)
	}

	unitDir := filepath.Join(configDir, "systemd", "user")
	unitPath := filepath.Join(unitDir, "tmuxatlas.service")

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("could not create systemd user directory: %w", err)
	}

	cfg := serviceConfig{
		BinaryPath: binPath,
		ExecStart:  binPath + " server",
		Path:       os.Getenv("PATH"),
		PublicURL:  publicURL,
	}

	tmpl, err := template.New("systemd").Parse(systemdUnit)
	if err != nil {
		return fmt.Errorf("could not parse systemd template: %w", err)
	}

	f, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("could not create unit file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("could not write unit file: %w", err)
	}

	fmt.Printf("Wrote %s\n", unitPath)

	// Stop the pre-rename service so it cannot contend for the same port.
	legacyUnitPath := filepath.Join(unitDir, "guppi.service")
	if _, err := os.Stat(legacyUnitPath); err == nil {
		_ = exec.CommandContext(ctx, "systemctl", "--user", "disable", "--now", "guppi.service").Run()
		fmt.Printf("Disabled legacy service %s (file retained for rollback)\n", legacyUnitPath)
	}

	// Reload and enable
	if err := exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w", err)
	}

	if err := exec.CommandContext(ctx, "systemctl", "--user", "enable", "--now", "tmuxatlas.service").Run(); err != nil {
		return fmt.Errorf("systemctl enable failed: %w", err)
	}

	fmt.Println("Service enabled and started (systemctl --user)")
	fmt.Println()
	fmt.Println("  Status:   systemctl --user status tmuxatlas")
	fmt.Println("  Logs:     journalctl --user -u tmuxatlas -f")
	fmt.Println("  Restart:  systemctl --user restart tmuxatlas")
	fmt.Printf("  Web UI:   %s\n", publicURL)
	return nil
}

func installDarwin(ctx context.Context, c *cli.Command) error {
	publicURL, err := validatePublicURL(c.String("public-url"))
	if err != nil {
		return err
	}
	binPath, err := getBinaryPath()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	plistPath := filepath.Join(agentDir, "com.tmuxatlas.server.plist")
	logDir := filepath.Join(home, "Library", "Logs")

	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("could not create LaunchAgents directory: %w", err)
	}

	cfg := serviceConfig{
		BinaryPath: binPath,
		Path:       os.Getenv("PATH"),
		LogDir:     logDir,
		PublicURL:  publicURL,
	}

	tmpl, err := template.New("launchd").Parse(launchdPlist)
	if err != nil {
		return fmt.Errorf("could not parse plist template: %w", err)
	}

	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("could not create plist file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("could not write plist file: %w", err)
	}

	fmt.Printf("Wrote %s\n", plistPath)

	// Stop the pre-rename service while retaining its plist for rollback.
	legacyPlistPath := filepath.Join(agentDir, "com.guppi.server.plist")
	if _, err := os.Stat(legacyPlistPath); err == nil {
		_ = exec.CommandContext(ctx, "launchctl", "unload", "-w", legacyPlistPath).Run()
		fmt.Printf("Unloaded legacy service %s (file retained for rollback)\n", legacyPlistPath)
	}

	// Load the agent
	if err := exec.CommandContext(ctx, "launchctl", "load", "-w", plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl load failed: %w", err)
	}

	fmt.Println("Service loaded and started (launchctl)")
	fmt.Println()
	fmt.Println("  Status:   launchctl list com.tmuxatlas.server")
	fmt.Printf("  Logs:     tail -f %s/tmuxatlas.stderr.log\n", cfg.LogDir)
	fmt.Printf("  Restart:  launchctl kickstart -k gui/$(id -u)/com.tmuxatlas.server\n")
	fmt.Printf("  Web UI:   %s\n", publicURL)
	return nil
}

func installExecute(ctx context.Context, c *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return installLinux(ctx, c)
	case "darwin":
		return installDarwin(ctx, c)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func uninstallLinux(ctx context.Context, c *cli.Command) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("could not determine config directory: %w", err)
	}

	unitPath := filepath.Join(configDir, "systemd", "user", "tmuxatlas.service")

	// Disable and stop
	_ = exec.CommandContext(ctx, "systemctl", "--user", "disable", "--now", "tmuxatlas.service").Run()

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not remove unit file: %w", err)
	}

	_ = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()

	fmt.Printf("Removed %s\n", unitPath)
	fmt.Println("Service disabled and stopped")
	return nil
}

func uninstallDarwin(ctx context.Context, c *cli.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.tmuxatlas.server.plist")

	// Unload the agent
	_ = exec.CommandContext(ctx, "launchctl", "unload", "-w", plistPath).Run()

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("could not remove plist file: %w", err)
	}

	fmt.Printf("Removed %s\n", plistPath)
	fmt.Println("Service unloaded and stopped")
	return nil
}

func uninstallExecute(ctx context.Context, c *cli.Command) error {
	switch runtime.GOOS {
	case "linux":
		return uninstallLinux(ctx, c)
	case "darwin":
		return uninstallDarwin(ctx, c)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func init() {
	cmd := &cli.Command{
		Name:  "install",
		Usage: "install TmuxAtlas as a user service for auto-start",
		Description: `Install TmuxAtlas to start automatically on login.

On Linux, installs a systemd user unit (~/.config/systemd/user/tmuxatlas.service).
On macOS, installs a launchd plist (~/Library/LaunchAgents/com.tmuxatlas.server.plist).

Use "tmuxatlas install" to install and enable, "tmuxatlas uninstall" to remove.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "public-url",
				Usage:   "Final browser-facing URL used for Passkeys",
				Sources: cli.EnvVars("TMUXATLAS_PUBLIC_URL"),
				Value:   "http://localhost:7654",
			},
		},
		Action: installExecute,
	}

	uninstallCmd := &cli.Command{
		Name:  "uninstall",
		Usage: "remove TmuxAtlas user service",
		Description: `Remove the TmuxAtlas auto-start service.

On Linux, disables and removes the systemd user unit.
On macOS, unloads and removes the launchd plist.`,
		Action: uninstallExecute,
	}

	common.RegisterCommand(cmd)
	common.RegisterCommand(uninstallCmd)
}
