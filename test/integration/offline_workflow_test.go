//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStateCommandsWorkWithoutBaseImage(t *testing.T) {
	h := newHarness(t)
	dir := h.writeProject("offline", "", nil)
	image := h.fakeImage(dir, "base.qcow2")
	path := filepath.Join(dir, "holos.yaml")
	if _, err := writeFile(dir, "holos.yaml", fmt.Sprintf("name: offline\nservices:\n  web:\n    image: %s\n", image)); err != nil {
		t.Fatal(err)
	}
	h.mustRun("up", "-f", path)
	if err := os.Remove(filepath.Join(dir, image)); err != nil {
		t.Fatal(err)
	}
	h.mustRun("ps", "-f", path)
	h.mustRun("logs", "-f", path)
	h.mustRun("stop", "-f", path, "web")
	h.mustRun("down", "-f", path)
}
