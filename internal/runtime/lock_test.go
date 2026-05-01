package runtime

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProjectLockNoWaitReportsHolder(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	holder := NewManager(stateDir)
	contender := NewManager(stateDir)
	contender.SetLockOptions(LockOptions{NoWait: true})

	err := holder.withProjectLock("demo", func() error {
		return contender.withProjectLock("demo", func() error {
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
	if !strings.Contains(busy.Owner, "pid=") {
		t.Fatalf("busy.Owner = %q, want pid metadata", busy.Owner)
	}
	if !strings.Contains(err.Error(), "--no-wait") {
		t.Fatalf("error should mention --no-wait, got %v", err)
	}
}

func TestProjectLockTimeout(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	holder := NewManager(stateDir)
	contender := NewManager(stateDir)
	contender.SetLockOptions(LockOptions{WaitTimeout: 30 * time.Millisecond})

	start := time.Now()
	err := holder.withProjectLock("demo", func() error {
		return contender.withProjectLock("demo", func() error {
			t.Fatal("contender acquired an already-held lock")
			return nil
		})
	})
	if err == nil {
		t.Fatal("expected lock timeout")
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatalf("lock returned before timeout elapsed: %s", time.Since(start))
	}

	var busy ProjectLockBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("error = %T %v, want ProjectLockBusyError", err, err)
	}
	if busy.NoWait {
		t.Fatalf("busy.NoWait = true, want false")
	}
	if busy.WaitTimeout != 30*time.Millisecond {
		t.Fatalf("busy.WaitTimeout = %s, want 30ms", busy.WaitTimeout)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error should mention timeout, got %v", err)
	}
}
