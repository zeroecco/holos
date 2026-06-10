package runtime

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/compose"
)

func (m *Manager) withProjectLock(projectName string, fn func() error) error {
	if err := compose.ValidateName(projectName); err != nil {
		return fmt.Errorf("invalid project name: %w", err)
	}
	if err := os.MkdirAll(m.stateDir, stateDirPerm); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.Chmod(m.stateDir, stateDirPerm); err != nil {
		return fmt.Errorf("tighten state dir: %w", err)
	}
	if err := os.MkdirAll(locksDir(m.stateDir), stateDirPerm); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}
	if err := os.Chmod(locksDir(m.stateDir), stateDirPerm); err != nil {
		return fmt.Errorf("tighten lock dir: %w", err)
	}
	lockPath := projectLockFile(m.stateDir, projectName)
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, projectLockPerm)
	if err != nil {
		return fmt.Errorf("open project lock %s: %w", lockPath, err)
	}
	defer file.Close()
	if err := lockProjectFile(file, lockPath, projectName, normalizeLockOptions(m.lockOptions)); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if err := writeProjectLockOwner(file); err != nil {
		return fmt.Errorf("write project lock %s: %w", lockPath, err)
	}
	return fn()
}

func lockProjectFile(file *os.File, lockPath, projectName string, opts LockOptions) error {
	deadline := time.Time{}
	if opts.WaitTimeout > 0 {
		deadline = time.Now().Add(opts.WaitTimeout)
	}

	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return fmt.Errorf("lock project %q: %w", projectName, err)
		}
		if shouldStopWaitingForLock(opts, deadline, time.Now()) {
			return ProjectLockBusyError{
				Project:     projectName,
				Path:        lockPath,
				Owner:       readProjectLockOwner(file),
				WaitTimeout: opts.WaitTimeout,
				NoWait:      opts.NoWait,
			}
		}
		time.Sleep(lockPollInterval)
	}
}

func shouldStopWaitingForLock(opts LockOptions, deadline time.Time, now time.Time) bool {
	return opts.NoWait || opts.WaitTimeout == 0 || (!deadline.IsZero() && now.After(deadline))
}
