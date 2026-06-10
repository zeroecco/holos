package runtime

import (
	"errors"
	"testing"
	"time"
)

const lockTestProjectName = "demo"

func TestProjectLockNoWaitReportsHolder(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	holder := NewManager(stateDir)
	contender := NewManager(stateDir)
	contender.SetLockOptions(LockOptions{NoWait: true})

	err := holder.withProjectLock(lockTestProjectName, func() error {
		return contender.withProjectLock(lockTestProjectName, func() error {
			t.Fatal("contender acquired an already-held lock")
			return nil
		})
	})
	if err == nil {
		t.Fatal("expected lock busy error")
	}

	var busy ProjectLockBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("error = %T %v, want ProjectLockBusyError", err, err)
	}
	if !busy.NoWait {
		t.Fatalf("busy.NoWait = false, want true")
	}
	assertStringContains(t, busy.Owner, "pid=")
	assertErrorContains(t, err, "--no-wait")
}

func TestProjectLockCreatesPrivateLockFile(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)

	if err := manager.withProjectLock(lockTestProjectName, func() error {
		return nil
	}); err != nil {
		t.Fatalf("withProjectLock: %v", err)
	}

	assertMode(t, stateDir, stateDirPerm)
	assertMode(t, locksDir(stateDir), stateDirPerm)
	assertMode(t, projectLockFile(stateDir, lockTestProjectName), projectLockPerm)
}

func TestProjectLockTimeout(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	holder := NewManager(stateDir)
	contender := NewManager(stateDir)
	waitTimeout := lockPollInterval + 30*time.Millisecond
	contender.SetLockOptions(LockOptions{WaitTimeout: waitTimeout})

	start := time.Now()
	err := holder.withProjectLock(lockTestProjectName, func() error {
		return contender.withProjectLock(lockTestProjectName, func() error {
			t.Fatal("contender acquired an already-held lock")
			return nil
		})
	})
	if err == nil {
		t.Fatal("expected lock timeout")
	}
	if time.Since(start) < waitTimeout {
		t.Fatalf("lock returned before timeout elapsed: %s", time.Since(start))
	}

	var busy ProjectLockBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("error = %T %v, want ProjectLockBusyError", err, err)
	}
	if busy.NoWait {
		t.Fatalf("busy.NoWait = true, want false")
	}
	if busy.WaitTimeout != waitTimeout {
		t.Fatalf("busy.WaitTimeout = %s, want %s", busy.WaitTimeout, waitTimeout)
	}
	assertErrorContains(t, err, "timed out")
}

func TestLockTimingConstants(t *testing.T) {
	t.Parallel()

	if DefaultLockWaitTimeout <= 0 {
		t.Fatalf("DefaultLockWaitTimeout = %s, want positive", DefaultLockWaitTimeout)
	}
	if lockPollInterval <= 0 || lockPollInterval >= DefaultLockWaitTimeout {
		t.Fatalf("lockPollInterval = %s, want positive and less than default wait %s", lockPollInterval, DefaultLockWaitTimeout)
	}
}

func TestNormalizeLockOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts LockOptions
		want LockOptions
	}{
		{name: "default wait", want: LockOptions{WaitTimeout: DefaultLockWaitTimeout}},
		{name: "custom wait", opts: LockOptions{WaitTimeout: time.Second}, want: LockOptions{WaitTimeout: time.Second}},
		{name: "no wait preserves zero timeout", opts: LockOptions{NoWait: true}, want: LockOptions{NoWait: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeLockOptions(tt.opts); got != tt.want {
				t.Fatalf("normalizeLockOptions = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestShouldStopWaitingForLock(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		opts     LockOptions
		deadline time.Time
		want     bool
	}{
		{name: "no wait", opts: LockOptions{NoWait: true}, want: true},
		{name: "no timeout", opts: LockOptions{}, want: true},
		{name: "before deadline", opts: LockOptions{WaitTimeout: time.Second}, deadline: now.Add(time.Second), want: false},
		{name: "at deadline", opts: LockOptions{WaitTimeout: time.Second}, deadline: now, want: false},
		{name: "after deadline", opts: LockOptions{WaitTimeout: time.Second}, deadline: now.Add(-time.Nanosecond), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldStopWaitingForLock(tt.opts, tt.deadline, now); got != tt.want {
				t.Fatalf("shouldStopWaitingForLock = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLockWaitDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		waitTimeout time.Duration
		noWait      bool
		want        string
	}{
		{name: "no wait", noWait: true, waitTimeout: time.Second, want: "--no-wait was set"},
		{name: "timeout", waitTimeout: 3 * time.Second, want: "timed out after 3s"},
		{name: "none", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := lockWaitDetail(tt.waitTimeout, tt.noWait); got != tt.want {
				t.Fatalf("lockWaitDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatProjectLockOwner(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 9, 12, 34, 56, 0, time.UTC)
	if got, want := formatProjectLockOwner(1234, startedAt), "pid=1234 started_at=2026-06-09T12:34:56Z\n"; got != want {
		t.Fatalf("formatProjectLockOwner = %q, want %q", got, want)
	}
}
