package socket

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
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
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("socket directory %s is not a real directory", dir)
	}
	if err := validateOwner(info, dir); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("set socket directory permissions: %w", err)
		}
		info, err = os.Lstat(dir)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0o700 {
			return fmt.Errorf("socket directory %s has permissions %04o; want 0700", dir, info.Mode().Perm())
		}
	}
	return nil
}

// Listen creates a private Unix socket owned by the current user. A stale
// socket may be replaced, but regular files, symlinks, and sockets owned by
// another user are never removed.
func Listen(socketPath string) (net.Listener, error) {
	if err := EnsureDir(socketPath); err != nil {
		return nil, err
	}
	if err := removeStaleSocket(socketPath); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("set socket permissions: %w", err)
	}
	if err := validateSocket(socketPath); err != nil {
		listener.Close()
		_ = os.Remove(socketPath)
		return nil, err
	}
	return listener, nil
}

// Cleanup removes a current-user Unix socket if it exists.
func Cleanup(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket path %s", socketPath)
	}
	if err := validateOwner(info, socketPath); err != nil {
		return err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeStaleSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace non-socket path %s", socketPath)
	}
	if err := validateOwner(info, socketPath); err != nil {
		return err
	}
	return os.Remove(socketPath)
}

func validateSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s is not a Unix socket", socketPath)
	}
	if err := validateOwner(info, socketPath); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("socket %s has permissions %04o; want 0600", socketPath, info.Mode().Perm())
	}
	return nil
}

func validateOwner(info os.FileInfo, path string) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot determine owner of %s", path)
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("%s is owned by uid %d; current uid is %d", path, stat.Uid, os.Getuid())
	}
	return nil
}
