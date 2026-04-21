package runtime

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureLayoutPermissions pins the on-disk mode of the state
// hierarchy to 0700. The tree contains generated SSH private keys,
// project records, and cloud-init seed material; world- or group-
// readable modes here would leak credentials and per-instance secrets
// to other local users.
func TestEnsureLayoutPermissions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if err := m.ensureLayout(); err != nil {
		t.Fatalf("ensureLayout: %v", err)
	}

	for _, sub := range []string{"", "projects", "instances"} {
		path := filepath.Join(dir, sub)
		assertMode(t, path, 0o700)
	}
}

// TestEnsureLayoutTightensExisting verifies the migration path: a
// state directory created by an older holos at 0755 is silently
// chmod'd back to 0700 on the next invocation.
func TestEnsureLayoutTightensExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatalf("seed loose dir: %v", err)
	}
	if err := os.Chmod(filepath.Join(dir, "projects"), 0o755); err != nil {
		t.Fatalf("chmod loose dir: %v", err)
	}

	m := NewManager(dir)
	if err := m.ensureLayout(); err != nil {
		t.Fatalf("ensureLayout: %v", err)
	}
	assertMode(t, filepath.Join(dir, "projects"), 0o700)
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}
