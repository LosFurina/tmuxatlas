package paths

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnv loads ~/.config/tmuxatlas/.env without overriding variables already
// present in the process environment. Only TmuxAtlas variables are accepted.
func LoadEnv() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".env")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNumber)
		}
		if !strings.HasPrefix(key, "TMUXATLAS_") {
			return fmt.Errorf("%s:%d: unsupported variable %s", path, lineNumber, key)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("%s:%d: %w", path, lineNumber, err)
			}
		}
	}
	return scanner.Err()
}

// SaveEnvValue atomically updates one TmuxAtlas variable in the user config.
func SaveEnvValue(key, value string) error {
	if !strings.HasPrefix(key, "TMUXATLAS_") || strings.ContainsAny(key, "=\r\n") {
		return fmt.Errorf("unsupported environment key %q", key)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("environment value must be a single line")
	}
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, ".env")
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	output := make([]string, 0, len(lines)+1)
	prefix := key + "="
	for _, line := range lines {
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		output = append(output, line)
	}
	output = append(output, prefix+value)

	temp, err := os.CreateTemp(dir, ".env-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(strings.Join(output, "\n") + "\n"); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
