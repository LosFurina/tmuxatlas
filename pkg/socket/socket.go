package socket

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const socketName = "tmuxatlas.sock"

// DefaultPath returns the default Unix socket path for the current user.
// It follows the XDG Base Directory Specification with platform-appropriate fallbacks:
//  1. $XDG_RUNTIME_DIR/tmuxatlas/tmuxatlas.sock (Linux standard)
//  2. $TMPDIR/tmuxatlas-$UID/tmuxatlas.sock (macOS / fallback)
//  3. /tmp/tmuxatlas-$UID/tmuxatlas.sock (last resort)
func DefaultPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "tmuxatlas", socketName)
	}

	uid := fmt.Sprintf("%d", os.Getuid())

	if runtime.GOOS == "darwin" {
		if tmpDir := os.Getenv("TMPDIR"); tmpDir != "" {
			return filepath.Join(tmpDir, "tmuxatlas-"+uid, socketName)
		}
	}

	return filepath.Join("/tmp", "tmuxatlas-"+uid, socketName)
}

// EnsureDir creates the parent directory for the socket path with 0700 permissions.
func EnsureDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	return os.MkdirAll(dir, 0700)
}

// Cleanup removes the socket file if it exists.
func Cleanup(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
