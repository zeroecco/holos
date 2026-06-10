package runtime

import (
	"path/filepath"
	"testing"
)

func TestNewManagerUsesDefaultLockOptions(t *testing.T) {
	t.Parallel()

	manager := NewManager("/tmp/holos-state")
	if manager.stateDir != "/tmp/holos-state" {
		t.Fatalf("stateDir = %q, want constructor input", manager.stateDir)
	}
	if manager.lockOptions != DefaultLockOptions() {
		t.Fatalf("lockOptions = %+v, want %+v", manager.lockOptions, DefaultLockOptions())
	}
}

func TestDefaultStateDirUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("HOLOS_STATE_DIR", "/tmp/holos-state")

	if got := DefaultStateDir(); got != "/tmp/holos-state" {
		t.Fatalf("DefaultStateDir() = %q, want environment override", got)
	}
}

func TestUserStateDir(t *testing.T) {
	t.Parallel()

	got := userStateDir("/home/alice")
	want := filepath.FromSlash("/home/alice/.local/state/holos")
	if got != want {
		t.Fatalf("userStateDir() = %q, want %q", got, want)
	}
}
