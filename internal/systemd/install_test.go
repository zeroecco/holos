package systemd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemctlWarning(t *testing.T) {
	t.Parallel()

	got := systemctlWarning("enable", errors.New("exit status 1"))
	want := "enable: exit status 1"
	if got != want {
		t.Fatalf("systemctlWarning = %q, want %q", got, want)
	}
}

func TestUninstallReportsRemoveFailure(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(xdgConfigHomeEnv, xdg)
	t.Setenv("PATH", t.TempDir())

	path, err := UnitPath(ScopeUser, testUnitProject)
	if err != nil {
		t.Fatalf("unit path: %v", err)
	}
	if err := os.MkdirAll(path, systemdUnitDirPerm); err != nil {
		t.Fatalf("create unit path directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("busy"), systemdUnitFilePerm); err != nil {
		t.Fatalf("create child file: %v", err)
	}

	res, err := Uninstall(ScopeUser, testUnitProject)
	if err == nil {
		t.Fatal("Uninstall err = nil, want remove failure")
	}
	mustContain(t, err.Error(), "remove unit:")
	if res.UnitPath != path {
		t.Fatalf("UnitPath = %q, want %q", res.UnitPath, path)
	}
}
