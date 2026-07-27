package paths

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// AppName is the canonical on-disk name for TmuxAtlas.
	AppName = "tmuxatlas"
	// legacyAppName is retained only to migrate data from releases named guppi.
	legacyAppName = "guppi"
)

// ConfigDir returns the TmuxAtlas configuration directory. Existing data from
// ~/.config/guppi is copied on first use without deleting the rollback source.
func ConfigDir() (string, error) {
	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); xdgDir != "" {
		return ensureMigratedDir(
			filepath.Join(xdgDir, AppName),
			filepath.Join(xdgDir, legacyAppName),
		)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return ensureMigratedDir(
		filepath.Join(home, ".config", AppName),
		filepath.Join(home, ".config", legacyAppName),
	)
}

// DataDir returns the TmuxAtlas application-data directory and migrates any
// legacy VAPID data without removing the old copy.
func DataDir() (string, error) {
	if xdgDir := os.Getenv("XDG_DATA_HOME"); xdgDir != "" {
		return ensureMigratedDir(
			filepath.Join(xdgDir, AppName),
			filepath.Join(xdgDir, legacyAppName),
		)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		return ensureMigratedDir(
			filepath.Join(base, AppName),
			filepath.Join(base, legacyAppName),
		)
	}
	base := filepath.Join(home, ".local", "share")
	return ensureMigratedDir(
		filepath.Join(base, AppName),
		filepath.Join(base, legacyAppName),
	)
}

func ensureMigratedDir(target, legacy string) (string, error) {
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", fmt.Errorf("create %s directory: %w", AppName, err)
	}

	info, err := os.Stat(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return target, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect legacy configuration: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("legacy path %s is not a directory", legacy)
	}
	if err := copyMissingFiles(legacy, target); err != nil {
		return "", fmt.Errorf("migrate legacy configuration from %s: %w", legacy, err)
	}
	return target, nil
}

func copyMissingFiles(source, target string) error {
	return filepath.WalkDir(source, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		targetPath := filepath.Join(target, relative)

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if !entry.Type().IsRegular() {
			return nil
		}

		sourceFile, err := os.Open(sourcePath)
		if err != nil {
			return err
		}

		targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if errors.Is(err, os.ErrExist) {
			sourceFile.Close()
			return nil
		}
		if err != nil {
			sourceFile.Close()
			return err
		}
		if _, err := io.Copy(targetFile, sourceFile); err != nil {
			sourceFile.Close()
			targetFile.Close()
			_ = os.Remove(targetPath)
			return err
		}
		if err := sourceFile.Close(); err != nil {
			targetFile.Close()
			return err
		}
		return targetFile.Close()
	})
}
