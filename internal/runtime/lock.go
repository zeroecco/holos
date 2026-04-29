package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/zeroecco/holos/internal/compose"
)

func locksDir(root string) string {
	return filepath.Join(root, "locks")
}

func projectLockFile(root, name string) string {
	return filepath.Join(locksDir(root), name+".lock")
}

func (m *Manager) withProjectLock(projectName string, fn func() error) error {
	if err := compose.ValidateName(projectName); err != nil {
		return fmt.Errorf("invalid project name: %w", err)
	}
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(m.stateDir, 0o700); err != nil {
		return fmt.Errorf("tighten state dir: %w", err)
	}
	if err := os.MkdirAll(locksDir(m.stateDir), 0o700); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	if err := os.Chmod(locksDir(m.stateDir), 0o700); err != nil {
		return fmt.Errorf("tighten lock dir: %w", err)
	}
	lockPath := projectLockFile(m.stateDir, projectName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open project lock %s: %w", lockPath, err)
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock project %q: %w", projectName, err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return fn()
}
