//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdatePreservesUnchangedService(t *testing.T) {
	h := newHarness(t)
	dir := h.writeProject("selective", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	source := fmt.Sprintf(`name: selective
services:
  db:
    image: %s
  web:
    image: %s
    vm:
      memory_mb: 256
`, img, img)
	path := filepath.Join(dir, "holos.yaml")
	if _, err := writeFile(dir, "holos.yaml", source); err != nil {
		t.Fatal(err)
	}
	h.mustRun("up", "-f", path)
	// Starting a selected service must not treat its peers as removals.
	h.mustRun("stop", "-f", path, "web")
	h.mustRun("start", "-f", path, "web")

	before := findProject(t, psList(t, h), "selective")
	pids := map[string]int{}
	markers := map[string]string{}
	for _, svc := range before.Services {
		inst := svc.Instances[0]
		pids[svc.Name] = inst.PID
		markers[svc.Name] = filepath.Join(inst.WorkDir, "development-work")
		if err := os.WriteFile(markers[svc.Name], []byte("keep me"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writeFile(dir, "holos.yaml", strings.Replace(source, "memory_mb: 256", "memory_mb: 512", 1)); err != nil {
		t.Fatal(err)
	}
	_, preview := h.mustRun("up", "--dry-run", "-f", path)
	assertContains(t, preview, "db: keep", "unchanged service preview")
	assertContains(t, preview, "web: replace", "changed service preview")
	if _, stderr, err := h.run("up", "-f", path); err == nil {
		t.Fatal("destructive update succeeded without --recreate")
	} else {
		assertContains(t, stderr, "--recreate", "replacement gate")
	}
	for _, marker := range markers {
		if _, err := os.Stat(marker); err != nil {
			t.Fatal("preview/refusal removed disk", err)
		}
	}
	h.mustRun("up", "--recreate", "-f", path)
	after := findProject(t, psList(t, h), "selective")
	for _, svc := range after.Services {
		if svc.Name == "db" && svc.Instances[0].PID != pids["db"] {
			t.Fatal("unchanged VM restarted")
		}
		if svc.Name == "web" && svc.Instances[0].PID == pids["web"] {
			t.Fatal("changed VM was not replaced")
		}
	}
	if _, err := os.Stat(markers["db"]); err != nil {
		t.Fatal("unchanged VM lost disk contents", err)
	}
	if _, err := os.Stat(markers["web"]); !os.IsNotExist(err) {
		t.Fatal("replacement retained old instance directory", err)
	}
	// Generated SSH keys must not make the next up look like a config change.
	h.mustRun("up", "-f", path)
	// A replacement failing to launch must not erase the unchanged service's
	// tracking record. Retry should create the missing service without another
	// destructive authorization.
	failing := strings.Replace(source, "memory_mb: 256", "memory_mb: 768", 1)
	if _, err := writeFile(dir, "holos.yaml", failing); err != nil {
		t.Fatal(err)
	}
	h.extraEnv = append(h.extraEnv, "HOLOS_MOCK_QEMU_EXIT_AFTER=10ms")
	if _, _, err := h.run("up", "--recreate", "-f", path); err == nil {
		t.Fatal("expected replacement launch failure")
	}
	failed := findProject(t, psList(t, h), "selective")
	if len(failed.Services) != 1 || failed.Services[0].Name != "db" || failed.Services[0].Instances[0].PID != pids["db"] {
		t.Fatalf("lost unchanged service on failure: %+v", failed)
	}
	h.extraEnv = h.extraEnv[:len(h.extraEnv)-1]
	h.mustRun("up", "-f", path)
	if _, err := os.Stat(markers["db"]); err != nil {
		t.Fatal("failed update lost unchanged disk", err)
	}

}
