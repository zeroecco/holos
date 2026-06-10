package runtime

import (
	"path/filepath"
	"testing"
)

func TestLockPaths(t *testing.T) {
	root := filepath.FromSlash("state/holos")

	if got, want := locksDir(root), filepath.FromSlash("state/holos/locks"); got != want {
		t.Fatalf("locksDir = %q, want %q", got, want)
	}
	lockFile := filepath.FromSlash("state/holos/locks/demo.lock")
	if got := projectLockFile(root, lockTestProjectName); got != lockFile {
		t.Fatalf("projectLockFile = %q, want %q", got, lockFile)
	}
}
