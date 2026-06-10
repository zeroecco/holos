package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteProjectRemoved(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writeProjectRemoved(&out, "demo")
	if got, want := out.String(), "project \"demo\" removed\n"; got != want {
		t.Fatalf("project removed output = %q, want %q", got, want)
	}
}

func TestRunUpVerifiesImageLockBeforeLaunch(t *testing.T) {
	dir := t.TempDir()
	imagePath := writeTestFile(t, dir, "base.qcow2", "base-image", 0o644)
	composePath := writeTestFile(t, dir, "holos.yaml", "name: demo\nservices:\n  web:\n    image: ./base.qcow2\n", 0o644)
	lockfile := imageLockfile{
		Version: 1,
		Project: "demo",
		Images: []imageLockEntry{{
			Service:   "web",
			Path:      imagePath,
			Format:    "qcow2",
			SizeBytes: int64(len("base-image")),
			SHA256:    strings.Repeat("0", 64),
		}},
	}
	if err := writeImageLockfile(filepath.Join(dir, defaultImageLockName), lockfile); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	err := runUp([]string{"-f", composePath, "--state-dir", filepath.Join(dir, "state")})
	if err == nil || !strings.Contains(err.Error(), "image lockfile") || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("runUp err = %v, want image lock drift", err)
	}
}
