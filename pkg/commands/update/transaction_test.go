package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransactionalReplaceAndRollback(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "tmuxatlas")
	source := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged, err := stageExecutable(source, executable)
	if err != nil {
		t.Fatal(err)
	}
	store := &transactionStore{path: filepath.Join(dir, "transaction.json")}
	tx := updateTransaction{
		Phase: phaseStaged, Executable: executable, StagedPath: staged,
		BackupPath: executable + ".previous", PreviousVersion: "v1", TargetVersion: "v2",
	}
	if err := store.save(tx); err != nil {
		t.Fatal(err)
	}
	if err := replaceWithBackup(staged, executable, tx.BackupPath); err != nil {
		t.Fatal(err)
	}
	tx.Phase = phaseReplaced
	if err := store.save(tx); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, executable, "new")
	assertFileContent(t, tx.BackupPath, "old")
	if err := rollbackExecutable(&tx, store); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, executable, "old")
	loaded, err := store.load()
	if err != nil || loaded.Phase != phaseRolledBack {
		t.Fatalf("journal = %#v, %v", loaded, err)
	}
	info, err := os.Stat(store.path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestRecoverInterruptedUpdate(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged")
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := &transactionStore{path: filepath.Join(dir, "transaction.json")}
	if err := store.save(updateTransaction{Phase: phaseStaged, StagedPath: staged}); err != nil {
		t.Fatal(err)
	}
	if err := recoverInterrupted(store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged binary survived recovery: %v", err)
	}
}

func TestStagingAndReplaceFailuresPreserveCurrentExecutable(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "tmuxatlas")
	if err := os.WriteFile(executable, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := stageExecutable(filepath.Join(dir, "missing"), executable); err == nil {
		t.Fatal("missing verified binary was staged")
	}
	assertFileContent(t, executable, "current")
	if err := replaceWithBackup(filepath.Join(dir, "missing-staged"), executable, executable+".previous"); err == nil {
		t.Fatal("missing staged executable was replaced")
	}
	assertFileContent(t, executable, "current")
}

func TestWaitForServiceHealth(t *testing.T) {
	attempts := 0
	service := &serviceInfo{probe: func(context.Context) (*serviceHealth, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("starting")
		}
		return &serviceHealth{Ready: true, Role: "agent", Version: "v2"}, nil
	}, isActive: func(context.Context) bool { return true }, role: "agent"}
	health, err := waitForServiceHealth(t.Context(), service, "v2", time.Second)
	if err != nil || health.Role != "agent" {
		t.Fatalf("health = %#v, %v", health, err)
	}
	service.probe = func(context.Context) (*serviceHealth, error) {
		return &serviceHealth{Ready: true, Version: "v1"}, nil
	}
	if _, err := waitForServiceHealth(t.Context(), service, "v2", 10*time.Millisecond); err == nil {
		t.Fatal("version mismatch was accepted")
	}
}

func TestTransactionJournalPhases(t *testing.T) {
	store := &transactionStore{path: filepath.Join(t.TempDir(), "transaction.json")}
	for _, phase := range []transactionPhase{
		phaseStaged, phaseReplaced, phaseRestarted, phaseHealthy,
		phaseRollingBack, phaseRolledBack, phaseCommitted,
	} {
		if err := store.save(updateTransaction{Phase: phase}); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.load()
		if err != nil || loaded.Phase != phase {
			t.Fatalf("phase = %q, want %q (err %v)", loaded.Phase, phase, err)
		}
	}
}

func TestRestorePreviousVerifiesOldRelease(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "tmuxatlas")
	backup := executable + ".previous"
	if err := os.WriteFile(executable, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	store := &transactionStore{path: filepath.Join(dir, "transaction.json")}
	tx := &updateTransaction{
		Phase: phaseRestarted, Executable: executable, BackupPath: backup,
		PreviousVersion: "v1", TargetVersion: "v2",
	}
	restarted := false
	service := &serviceInfo{
		active: true, role: "agent",
		restart:  func(context.Context) error { restarted = true; return nil },
		isActive: func(context.Context) bool { return restarted },
		probe: func(context.Context) (*serviceHealth, error) {
			return &serviceHealth{Ready: restarted, Role: "agent", Version: "v1"}, nil
		},
	}
	if err := restorePrevious(t.Context(), tx, store, service); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, executable, "old")

	service.restart = func(context.Context) error { return errors.New("restart failed") }
	if err := restorePrevious(t.Context(), tx, store, service); err == nil {
		t.Fatal("rollback restart failure was hidden")
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("%s = %q, want %q", path, raw, want)
	}
}
