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
Description={{.Description}}
After=default.target

[Service]
Type=simple
ExecStart={{.ExecStart}}
Restart=on-failure
RestartSec=5
Environment=PATH={{.Path}}
Environment={{.EnvironmentName}}={{.EnvironmentValue}}

[Install]
WantedBy=default.target
`

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		<string>{{.Command}}</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogDir}}/{{.LogName}}.stdout.log</string>
	<key>StandardErrorPath</key>
	<string>{{.LogDir}}/{{.LogName}}.stderr.log</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>{{.Path}}</string>
		<key>{{.EnvironmentName}}</key>
		<string>{{.EnvironmentValue}}</string>
	</dict>
</dict>
</plist>
`

type serviceConfig struct {
	BinaryPath       string
	ExecStart        string
	Path             string
	LogDir           string
	Description      string
	Command          string
	Label            string
	LogName          string
	EnvironmentName  string
	EnvironmentValue string
}

type serviceRole struct {
	mode             string
	command          string
	systemdName      string
	launchdLabel     string
	description      string
	environmentName  string
	environmentValue string
}

func roleFromCommand(c *cli.Command) (serviceRole, error) {
	switch c.String("mode") {
	case "server", "hub":
		publicURL, err := validatePublicURL(c.String("public-url"))
		if err != nil {
			return serviceRole{}, err
		}
		return serviceRole{
			mode: "server", command: "server", systemdName: "tmuxatlas.service",
			launchdLabel:    "com.tmuxatlas.server",
			description:     "TmuxAtlas - Web dashboard for tmux sessions",
			environmentName: "TMUXATLAS_PUBLIC_URL", environmentValue: publicURL,
		}, nil
	case "agent":
		hubURL, err := validatePublicURL(c.String("hub"))
		if err != nil {
			return serviceRole{}, fmt.Errorf("hub URL: %w", err)
		}
		return serviceRole{
			mode: "agent", command: "agent", systemdName: "tmuxatlas-agent.service",
			launchdLabel:    "com.tmuxatlas.agent",
			description:     "TmuxAtlas - Headless tmux agent",
			environmentName: "TMUXATLAS_HUB", environmentValue: hubURL,
		}, nil
	default:
		return serviceRole{}, fmt.Errorf("mode must be server or agent")
	}
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
	role, err := roleFromCommand(c)
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
	unitPath := filepath.Join(unitDir, role.systemdName)

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("could not create systemd user directory: %w", err)
	}

	cfg := serviceConfig{
		BinaryPath: binPath, ExecStart: binPath + " " + role.command,
		Path: os.Getenv("PATH"), Description: role.description,
		EnvironmentName: role.environmentName, EnvironmentValue: role.environmentValue,
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

	otherService := "tmuxatlas.service"
	if role.mode == "server" {
		otherService = "tmuxatlas-agent.service"
	}
	_ = exec.CommandContext(ctx, "systemctl", "--user", "disable", "--now", otherService).Run()

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

	if err := exec.CommandContext(ctx, "systemctl", "--user", "enable", "--now", role.systemdName).Run(); err != nil {
		return fmt.Errorf("systemctl enable failed: %w", err)
	}

	fmt.Println("Service enabled and started (systemctl --user)")
	fmt.Println()
	fmt.Printf("  Status:   systemctl --user status %s\n", role.systemdName)
	fmt.Printf("  Logs:     journalctl --user -u %s -f\n", role.systemdName)
	fmt.Printf("  Restart:  systemctl --user restart %s\n", role.systemdName)
	if role.mode == "server" {
		fmt.Printf("  Web UI:   %s\n", role.environmentValue)
	} else {
		fmt.Printf("  Hub:      %s\n", role.environmentValue)
	}
	return nil
}

func installDarwin(ctx context.Context, c *cli.Command) error {
	role, err := roleFromCommand(c)
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
	plistPath := filepath.Join(agentDir, role.launchdLabel+".plist")
	logDir := filepath.Join(home, "Library", "Logs")

	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("could not create LaunchAgents directory: %w", err)
	}

	cfg := serviceConfig{
		BinaryPath: binPath, Path: os.Getenv("PATH"), LogDir: logDir,
		Command: role.command, Label: role.launchdLabel,
		LogName: role.launchdLabel, EnvironmentName: role.environmentName,
		EnvironmentValue: role.environmentValue,
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

	otherLabel := "com.tmuxatlas.server"
	if role.mode == "server" {
		otherLabel = "com.tmuxatlas.agent"
	}
	otherPath := filepath.Join(agentDir, otherLabel+".plist")
	_ = exec.CommandContext(ctx, "launchctl", "unload", "-w", otherPath).Run()

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
	fmt.Printf("  Status:   launchctl list %s\n", role.launchdLabel)
	fmt.Printf("  Logs:     tail -f %s/%s.stderr.log\n", cfg.LogDir, cfg.LogName)
	fmt.Printf("  Restart:  launchctl kickstart -k gui/$(id -u)/%s\n", role.launchdLabel)
	if role.mode == "server" {
		fmt.Printf("  Web UI:   %s\n", role.environmentValue)
	} else {
		fmt.Printf("  Hub:      %s\n", role.environmentValue)
	}
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

	serviceName := "tmuxatlas.service"
	if c.String("mode") == "agent" {
		serviceName = "tmuxatlas-agent.service"
	}
	unitPath := filepath.Join(configDir, "systemd", "user", serviceName)

	// Disable and stop
	_ = exec.CommandContext(ctx, "systemctl", "--user", "disable", "--now", serviceName).Run()

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

	label := "com.tmuxatlas.server"
	if c.String("mode") == "agent" {
		label = "com.tmuxatlas.agent"
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", label+".plist")

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

Server mode installs tmuxatlas.service or com.tmuxatlas.server.
Agent mode installs tmuxatlas-agent.service or com.tmuxatlas.agent and opens no TCP listener.

Use "tmuxatlas install" to install and enable, "tmuxatlas uninstall" to remove.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "mode", Usage: "Service role: server or agent",
				Value: "server", Sources: cli.EnvVars("TMUXATLAS_ROLE"),
			},
			&cli.StringFlag{
				Name:    "public-url",
				Usage:   "Final browser-facing URL used for Passkeys",
				Sources: cli.EnvVars("TMUXATLAS_PUBLIC_URL"),
				Value:   "http://localhost:7654",
			},
			&cli.StringFlag{
				Name: "hub", Usage: "Trusted Hub URL for agent mode",
				Sources: cli.EnvVars("TMUXATLAS_HUB"),
			},
		},
		Action: installExecute,
	}

	uninstallCmd := &cli.Command{
		Name:  "uninstall",
		Usage: "remove TmuxAtlas user service",
		Description: `Remove the TmuxAtlas auto-start service.

Select the server or agent service with --mode.`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "mode", Usage: "Service role: server or agent", Value: "server"},
		},
		Action: uninstallExecute,
	}

	common.RegisterCommand(cmd)
	common.RegisterCommand(uninstallCmd)
}
