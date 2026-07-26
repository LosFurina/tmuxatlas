package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/LosFurina/tmuxatlas/pkg/paths"
)

type transactionPhase string

const (
	phaseStaged      transactionPhase = "staged"
	phaseReplaced    transactionPhase = "replaced"
	phaseRestarted   transactionPhase = "restarted"
	phaseHealthy     transactionPhase = "healthy"
	phaseRollingBack transactionPhase = "rolling-back"
	phaseRolledBack  transactionPhase = "rolled-back"
	phaseCommitted   transactionPhase = "committed"
)

type updateTransaction struct {
	Phase           transactionPhase `json:"phase"`
	Executable      string           `json:"executable"`
	StagedPath      string           `json:"staged_path,omitempty"`
	BackupPath      string           `json:"backup_path,omitempty"`
	PreviousVersion string           `json:"previous_version"`
	TargetVersion   string           `json:"target_version"`
	Service         string           `json:"service,omitempty"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Error           string           `json:"error,omitempty"`
}

type transactionStore struct{ path string }

func defaultTransactionStore() (*transactionStore, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	return &transactionStore{path: filepath.Join(dir, "update-transaction.json")}, nil
}

func (s *transactionStore) save(tx updateTransaction) error {
	tx.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.path, append(raw, '\n'), 0o600)
}

func (s *transactionStore) load() (*updateTransaction, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tx updateTransaction
	if err := json.Unmarshal(raw, &tx); err != nil {
		return nil, fmt.Errorf("decode update transaction: %w", err)
	}
	return &tx, nil
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func stageExecutable(source, executable string) (string, error) {
	staged, err := os.CreateTemp(filepath.Dir(executable), ".tmuxatlas-staged-*")
	if err != nil {
		return "", fmt.Errorf("stage beside executable: %w", err)
	}
	path := staged.Name()
	if err := staged.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	if err := copyExecutable(source, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func replaceWithBackup(staged, executable, backup string) error {
	backupTemp := backup + ".tmp"
	_ = os.Remove(backupTemp)
	if err := copyExecutable(executable, backupTemp); err != nil {
		return fmt.Errorf("backup current executable: %w", err)
	}
	if err := os.Rename(backupTemp, backup); err != nil {
		_ = os.Remove(backupTemp)
		return fmt.Errorf("publish backup: %w", err)
	}
	if err := os.Rename(staged, executable); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}

func rollbackExecutable(tx *updateTransaction, store *transactionStore) error {
	if tx == nil || tx.BackupPath == "" {
		return errors.New("no previous release is available")
	}
	tx.Phase = phaseRollingBack
	if err := store.save(*tx); err != nil {
		return err
	}
	staged, err := stageExecutable(tx.BackupPath, tx.Executable)
	if err != nil {
		tx.Error = err.Error()
		_ = store.save(*tx)
		return err
	}
	if err := os.Rename(staged, tx.Executable); err != nil {
		tx.Error = err.Error()
		_ = store.save(*tx)
		return err
	}
	tx.Phase = phaseRolledBack
	tx.Error = ""
	return store.save(*tx)
}

func recoverInterrupted(store *transactionStore) error {
	tx, err := store.load()
	if err != nil || tx == nil {
		return err
	}
	switch tx.Phase {
	case phaseStaged:
		if tx.StagedPath != "" {
			_ = os.Remove(tx.StagedPath)
		}
		tx.Phase = phaseRolledBack
		return store.save(*tx)
	case phaseReplaced, phaseRestarted, phaseRollingBack:
		return rollbackExecutable(tx, store)
	case phaseHealthy:
		tx.Phase = phaseCommitted
		return store.save(*tx)
	default:
		return nil
	}
}

func restorePrevious(ctx context.Context, tx *updateTransaction, store *transactionStore, service *serviceInfo) error {
	if err := rollbackExecutable(tx, store); err != nil {
		return err
	}
	if service == nil || !service.active {
		return nil
	}
	if err := service.restart(ctx); err != nil {
		return fmt.Errorf("restart previous release: %w", err)
	}
	if _, err := waitForServiceHealth(ctx, service, tx.PreviousVersion, 30*time.Second); err != nil {
		return fmt.Errorf("previous release did not become ready: %w", err)
	}
	return nil
}
