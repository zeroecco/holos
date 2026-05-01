package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/compose"
)

// DefaultLockWaitTimeout bounds how long a lifecycle command waits for
// another holos process that is already mutating the same project.
const DefaultLockWaitTimeout = 5 * time.Minute

// LockOptions controls how project-scoped lifecycle locks are acquired.
type LockOptions struct {
	WaitTimeout time.Duration
	NoWait      bool
}

// DefaultLockOptions returns the default bounded lock wait behavior.
func DefaultLockOptions() LockOptions {
	return LockOptions{WaitTimeout: DefaultLockWaitTimeout}
}

// SetLockOptions applies lock acquisition behavior to this manager.
func (m *Manager) SetLockOptions(opts LockOptions) {
	m.lockOptions = normalizeLockOptions(opts)
}

func normalizeLockOptions(opts LockOptions) LockOptions {
	if opts.WaitTimeout == 0 && !opts.NoWait {
		opts.WaitTimeout = DefaultLockWaitTimeout
	}
	return opts
}

// ProjectLockBusyError reports that another holos process already holds the
// project lock. The lock file contents are advisory diagnostics; the kernel
// flock is the source of truth.
type ProjectLockBusyError struct {
	Project     string
	Path        string
	Owner       string
	WaitTimeout time.Duration
	NoWait      bool
}

func (e ProjectLockBusyError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "project %q is locked by another holos process", e.Project)
	if e.Owner != "" {
		fmt.Fprintf(&b, " (%s)", e.Owner)
	}
	fmt.Fprintf(&b, "; lock file: %s", e.Path)
	if e.NoWait {
		b.WriteString("; --no-wait was set")
	} else if e.WaitTimeout > 0 {
		fmt.Fprintf(&b, "; timed out after %s", e.WaitTimeout)
	}
	b.WriteString("; wait for the other command to finish or retry with a longer --lock-timeout")
	return b.String()
}

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
		if opts.NoWait || opts.WaitTimeout == 0 || (!deadline.IsZero() && time.Now().After(deadline)) {
			return ProjectLockBusyError{
				Project:     projectName,
				Path:        lockPath,
				Owner:       readProjectLockOwner(file),
				WaitTimeout: opts.WaitTimeout,
				NoWait:      opts.NoWait,
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func writeProjectLockOwner(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	_, err := fmt.Fprintf(file, "pid=%d started_at=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	return err
}

func readProjectLockOwner(file *os.File) string {
	if _, err := file.Seek(0, 0); err != nil {
		return ""
	}
	payload, err := os.ReadFile(file.Name())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}
