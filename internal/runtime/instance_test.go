package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestCreateOverlayIncludesConfiguredDiskSize(t *testing.T) {
	dir := t.TempDir()
	qemuImg := filepath.Join(dir, "qemu-img")
	logPath := filepath.Join(dir, "qemu-img.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HOLOS_QEMU_IMG_LOG\"\n"
	if err := os.WriteFile(qemuImg, []byte(script), 0o755); err != nil {
		t.Fatalf("write qemu-img mock: %v", err)
	}
	t.Setenv("HOLOS_QEMU_IMG", qemuImg)
	t.Setenv("HOLOS_QEMU_IMG_LOG", logPath)

	m := &Manager{}
	manifest := config.Manifest{
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			DiskSizeBytes: 2 * (1 << 30),
		},
	}
	overlayPath := filepath.Join(dir, "root.qcow2")
	if err := m.createOverlay(manifest, overlayPath); err != nil {
		t.Fatalf("createOverlay: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(args) < 2 || args[len(args)-2] != overlayPath || args[len(args)-1] != "2147483648" {
		t.Fatalf("expected overlay path followed by disk size, got %v", args)
	}
}
