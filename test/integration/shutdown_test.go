//go:build integration

package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestShutdown_GracefulPowerdown confirms `down` sends QMP system_powerdown
// before escalating to signals. The mock qemu logs an EVENT marker when it
// receives the powerdown command; the absence of a sigterm marker shows
// the runtime never needed the fallback.
func TestShutdown_GracefulPowerdown(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("graceful", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: graceful
services:
  svc:
    image: %s
    replicas: 2
    stop_grace_period: 5s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")
	_, downStderr := h.mustRun("down", "-f", dir+"/holos.yaml")

	logData, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu log: %v", err)
	}
	log := string(logData)

	powerdowns := strings.Count(log, "EVENT:qmp-powerdown")
	if powerdowns != 2 {
		t.Fatalf("expected 2 QMP powerdown events (one per replica); got %d\nholos down stderr:\n%s\nqemu log:\n%s",
			powerdowns, downStderr, log)
	}
	if strings.Contains(log, "EVENT:sigterm") {
		t.Fatalf("did not expect SIGTERM fallback on graceful down; got:\n%s", log)
	}
}

// TestShutdown_StopServiceUsesQMP verifies `stop <svc>` also goes through
// QMP (not just `down`), since both converge on stopInstance.
func TestShutdown_StopServiceUsesQMP(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("stopqmp", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: stopqmp
services:
  a:
    image: %s
  b:
    image: %s
`, img, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")
	h.mustRun("stop", "-f", dir+"/holos.yaml", "a")

	logData, _ := os.ReadFile(h.qemuLog)
	log := string(logData)
	if !strings.Contains(log, "EVENT:qmp-powerdown") {
		t.Fatalf("expected QMP powerdown during `stop a`; got:\n%s", log)
	}
}

// TestShutdown_GracePeriodPersisted confirms the compose field is parsed
// and stored in the persisted project record so stop paths see it.
func TestShutdown_GracePeriodPersisted(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("grace", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: grace
services:
  svc:
    image: %s
    stop_grace_period: 45s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	proj := findProject(t, psList(t, h), "grace")
	if len(proj.Services) == 0 || len(proj.Services[0].Instances) == 0 {
		t.Fatal("missing service or instance")
	}
	inst := proj.Services[0].Instances[0]
	if inst.StopGracePeriodSec != 45 {
		t.Fatalf("expected stop_grace_period_sec=45; got %d", inst.StopGracePeriodSec)
	}
}

// TestShutdown_InvalidDurationRejected ensures a bad duration string
// surfaces a clear error during validate rather than at stop time.
func TestShutdown_InvalidDurationRejected(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("badgrace", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: badgrace
services:
  svc:
    image: %s
    stop_grace_period: forever
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to reject malformed stop_grace_period")
	}
	if !strings.Contains(stderr, "stop_grace_period") {
		t.Fatalf("expected error to mention stop_grace_period; got:\n%s", stderr)
	}
}
