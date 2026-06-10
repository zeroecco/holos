package runtime

import (
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultLockWaitTimeout bounds how long a lifecycle command waits for
	// another holos process that is already mutating the same project.
	DefaultLockWaitTimeout = 5 * time.Minute

	// lockPollInterval is the retry cadence while waiting for another process
	// to release a project lock.
	lockPollInterval = 100 * time.Millisecond
)

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
	if detail := lockWaitDetail(e.WaitTimeout, e.NoWait); detail != "" {
		fmt.Fprintf(&b, "; %s", detail)
	}
	b.WriteString("; wait for the other command to finish or retry with a longer --lock-timeout")
	return b.String()
}

func lockWaitDetail(waitTimeout time.Duration, noWait bool) string {
	if noWait {
		return "--no-wait was set"
	}
	if waitTimeout > 0 {
		return fmt.Sprintf("timed out after %s", waitTimeout)
	}
	return ""
}
