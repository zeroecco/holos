package runtime

import (
	"path/filepath"
	"testing"
)

func TestNewInstancePaths(t *testing.T) {
	workDir := testPathWorkDir(0)

	paths := newInstancePaths(workDir)

	assertPath(t, "overlay", paths.overlay, "state/holos/instances/demo/web-0/root.qcow2")
	assertPath(t, "consoleLog", paths.consoleLog, "state/holos/instances/demo/web-0/console.log")
	assertPath(t, "serialSocket", paths.serialSocket, "state/holos/instances/demo/web-0/serial.sock")
	assertPath(t, "qmpSocket", paths.qmpSocket, "state/holos/instances/demo/web-0/qmp.sock")
	assertPath(t, "qemuLog", paths.qemuLog, "state/holos/instances/demo/web-0/qemu.log")
	assertPath(t, "ovmfVars", paths.ovmfVars, "state/holos/instances/demo/web-0/OVMF_VARS.fd")
}

func assertPath(t *testing.T, name, got, wantSlash string) {
	t.Helper()

	want := filepath.FromSlash(wantSlash)
	if got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}
