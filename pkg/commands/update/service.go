package update

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type serviceInfo struct {
	kind       string
	name       string
	definition string
	executable string
	active     bool
	restart    func(context.Context) error
}

func parseSystemdExecStart(raw string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		if value == "" {
			return "", errors.New("systemd ExecStart is empty")
		}
		if value[0] == '"' {
			quoted := value
			if index := strings.Index(value[1:], "\""); index >= 0 {
				quoted = value[:index+2]
			}
			path, err := strconv.Unquote(quoted)
			if err != nil {
				return "", fmt.Errorf("parse quoted systemd ExecStart: %w", err)
			}
			return path, nil
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return "", errors.New("systemd ExecStart is empty")
		}
		return fields[0], nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("systemd service has no ExecStart")
}

func parseLaunchdExecutable(raw []byte) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(raw)))
	wantArguments := false
	inArguments := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "key":
				var key string
				if err := decoder.DecodeElement(&key, &element); err != nil {
					return "", err
				}
				wantArguments = key == "ProgramArguments"
			case "array":
				if wantArguments {
					inArguments = true
					wantArguments = false
				}
			case "string":
				if inArguments {
					var value string
					if err := decoder.DecodeElement(&value, &element); err != nil {
						return "", err
					}
					if value == "" {
						return "", errors.New("launchd ProgramArguments executable is empty")
					}
					return value, nil
				}
			}
		case xml.EndElement:
			if element.Name.Local == "array" && inArguments {
				inArguments = false
			}
		}
	}
	return "", errors.New("launchd plist has no ProgramArguments executable")
}

func discoverSystemdService() (*serviceInfo, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	var inactive *serviceInfo
	for _, name := range []string{"tmuxatlas-agent.service", "tmuxatlas.service"} {
		definition := filepath.Join(configDir, "systemd", "user", name)
		raw, readErr := os.ReadFile(definition)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		executable, parseErr := parseSystemdExecStart(string(raw))
		if parseErr != nil {
			return nil, fmt.Errorf("%s: %w", definition, parseErr)
		}
		active := exec.Command("systemctl", "--user", "is-active", "--quiet", name).Run() == nil
		serviceName := name
		service := &serviceInfo{
			kind: "systemd", name: serviceName, definition: definition,
			executable: executable, active: active,
			restart: func(ctx context.Context) error {
				output, restartErr := exec.CommandContext(ctx, "systemctl", "--user", "restart", serviceName).CombinedOutput()
				if restartErr != nil {
					return fmt.Errorf("systemctl restart: %w: %s", restartErr, strings.TrimSpace(string(output)))
				}
				return nil
			},
		}
		if active {
			return service, nil
		}
		if inactive == nil {
			inactive = service
		}
	}
	return inactive, nil
}

func discoverLaunchdService() (*serviceInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var inactive *serviceInfo
	for _, label := range []string{"com.tmuxatlas.agent", "com.tmuxatlas.server"} {
		definition := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
		raw, readErr := os.ReadFile(definition)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		executable, parseErr := parseLaunchdExecutable(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("%s: %w", definition, parseErr)
		}
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
		active := exec.Command("launchctl", "print", target).Run() == nil
		serviceLabel := label
		serviceTarget := target
		service := &serviceInfo{
			kind: "launchd", name: serviceLabel, definition: definition,
			executable: executable, active: active,
			restart: func(ctx context.Context) error {
				output, restartErr := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", serviceTarget).CombinedOutput()
				if restartErr != nil {
					return fmt.Errorf("launchctl kickstart: %w: %s", restartErr, strings.TrimSpace(string(output)))
				}
				return nil
			},
		}
		if active {
			return service, nil
		}
		if inactive == nil {
			inactive = service
		}
	}
	return inactive, nil
}

func discoverService() (*serviceInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return discoverSystemdService()
	case "darwin":
		return discoverLaunchdService()
	default:
		return nil, nil
	}
}

func resolvedPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path is not absolute: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func currentExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolvedPath(executable)
}

func validateServiceExecutable(service *serviceInfo, executable string) error {
	if service == nil {
		return nil
	}
	currentResolved, err := resolvedPath(executable)
	if err != nil {
		return fmt.Errorf("resolve current executable %s: %w", executable, err)
	}
	serviceExecutable, err := resolvedPath(service.executable)
	if err != nil {
		return fmt.Errorf("resolve %s executable %s: %w", service.kind, service.executable, err)
	}
	if serviceExecutable != currentResolved {
		return fmt.Errorf(
			"%s uses %s, but this command is %s; run the service binary directly to avoid updating the wrong copy",
			service.name, serviceExecutable, currentResolved,
		)
	}
	return nil
}
