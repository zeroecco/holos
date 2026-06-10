package runtime

import (
	"path/filepath"
	"testing"
)

func TestStateLayoutPaths(t *testing.T) {
	root := filepath.FromSlash(testPathRoot)

	if got, want := projectsDir(root), filepath.FromSlash("state/holos/projects"); got != want {
		t.Fatalf("projectsDir = %q, want %q", got, want)
	}
	if got, want := instancesRoot(root), filepath.FromSlash("state/holos/instances"); got != want {
		t.Fatalf("instancesRoot = %q, want %q", got, want)
	}
	if got, want := projectFile(root, testPathProject), filepath.FromSlash("state/holos/projects/demo.json"); got != want {
		t.Fatalf("projectFile = %q, want %q", got, want)
	}
	if got, want := projectRecordsGlob(root), filepath.FromSlash("state/holos/projects/*.json"); got != want {
		t.Fatalf("projectRecordsGlob = %q, want %q", got, want)
	}
	wantInstanceDir := filepath.FromSlash("state/holos/instances/demo/web-2")
	if got := projectInstanceDir(root, testPathProject, testPathService, testPathIndex); got != wantInstanceDir {
		t.Fatalf("projectInstanceDir = %q, want %q", got, wantInstanceDir)
	}
}

func TestStateLayoutDirs(t *testing.T) {
	root := filepath.FromSlash(testPathRoot)
	want := []string{
		root,
		filepath.FromSlash("state/holos/projects"),
		filepath.FromSlash("state/holos/instances"),
		filepath.FromSlash("state/holos/locks"),
	}
	assertStringSliceEqual(t, "stateLayoutDirs", stateLayoutDirs(root), want)
}
