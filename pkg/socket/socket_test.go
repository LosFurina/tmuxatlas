package socket

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ta-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestListenCreatesPrivateSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "runtime", "tmuxatlas.sock")
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	defer Cleanup(path)

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("%s is not a socket", path)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket permissions = %04o, want 0600", got)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("socket is accessible by group or others: %04o", got)
	}
}

func TestEnsureDirNormalizesBroadPermissions(t *testing.T) {
	dir := filepath.Join(shortTempDir(t), "runtime")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(dir, "tmuxatlas.sock")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %04o, want 0700", got)
	}
}

func TestListenRefusesToReplaceRegularFile(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "tmuxatlas.sock")
	if err := os.WriteFile(path, []byte("do not delete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path); err == nil {
		t.Fatal("Listen replaced a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "do not delete" {
		t.Fatalf("regular file changed: %q", data)
	}
}

func TestCleanupRejectsNonSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "tmuxatlas.sock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(path); err == nil {
		t.Fatal("Cleanup removed a non-socket")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("non-socket was removed: %v", err)
	}
}

func TestListenReplacesCurrentUserStaleSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "tmuxatlas.sock")
	first, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	if err := Cleanup(path); err != nil {
		t.Fatal(err)
	}
}
