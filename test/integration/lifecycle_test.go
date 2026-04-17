//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type psProject struct {
	Name     string `json:"name"`
	SpecHash string `json:"spec_hash"`
	Services []struct {
		Name            string `json:"name"`
		DesiredReplicas int    `json:"desired_replicas"`
		Instances       []struct {
			Name    string `json:"name"`
			Index   int    `json:"index"`
			PID     int    `json:"pid"`
			Status  string `json:"status"`
			WorkDir string `json:"work_dir"`
			Ports   []struct {
				HostPort  int    `json:"host_port"`
				GuestPort int    `json:"guest_port"`
				Protocol  string `json:"protocol"`
			} `json:"ports"`
		} `json:"instances"`
	} `json:"services"`
	Network struct {
		MulticastGroup string            `json:"multicast_group"`
		MulticastPort  int               `json:"multicast_port"`
		Subnet         string            `json:"subnet"`
		Hosts          map[string]string `json:"hosts"`
	} `json:"network"`
}

func psList(t *testing.T, h *harness) []psProject {
	t.Helper()
	stdout, _ := h.mustRun("ps", "-json")
	var projects []psProject
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("decode ps -json: %v\n%s", err, stdout)
	}
	return projects
}

func findProject(t *testing.T, projects []psProject, name string) psProject {
	t.Helper()
	for _, p := range projects {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("project %q not found in ps output; got %d projects", name, len(projects))
	return psProject{}
}

// TestLifecycle_UpStopStartDown is the workhorse end-to-end test: bring a
// two-service project up (3 instances), stop one service, start it again,
// and tear the whole thing down.
func TestLifecycle_UpStopStartDown(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("lifecycle", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: lifecycle
services:
  db:
    image: %s
    vm:
      memory_mb: 256
  web:
    image: %s
    replicas: 2
    depends_on: [db]
    ports:
      - "0:80"
`, img, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	stdout, _ := h.mustRun("up", "-f", dir+"/holos.yaml")
	assertContains(t, stdout, "project: lifecycle", "up output")
	assertContains(t, stdout, "service: db", "up listed db")
	assertContains(t, stdout, "service: web", "up listed web")
	assertContains(t, stdout, "2/2 running", "web should have 2 running replicas")

	// ps must reflect both services.
	projects := psList(t, h)
	proj := findProject(t, projects, "lifecycle")
	if len(proj.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(proj.Services))
	}

	runningTotal := 0
	for _, svc := range proj.Services {
		for _, inst := range svc.Instances {
			if inst.Status != "running" {
				t.Fatalf("expected instance %s running, got %q", inst.Name, inst.Status)
			}
			if inst.PID <= 0 {
				t.Fatalf("expected instance %s to have a pid", inst.Name)
			}
			runningTotal++
			if _, err := os.Stat(inst.WorkDir); err != nil {
				t.Fatalf("instance workdir missing: %v", err)
			}
			// web should have an auto-allocated port > 0.
			if svc.Name == "web" && len(inst.Ports) != 1 {
				t.Fatalf("web should have 1 port mapping; got %d", len(inst.Ports))
			}
			if svc.Name == "web" && inst.Ports[0].HostPort <= 0 {
				t.Fatalf("web host port should be auto-allocated; got %d", inst.Ports[0].HostPort)
			}
		}
	}
	if runningTotal != 3 {
		t.Fatalf("expected 3 total running instances (1 db + 2 web); got %d", runningTotal)
	}

	// Confirm qemu was actually invoked with expected arguments.
	qemuLog, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu mock log: %v", err)
	}
	logStr := string(qemuLog)
	assertContains(t, logStr, "-enable-kvm", "qemu invocation should include -enable-kvm")
	assertContains(t, logStr, "-netdev", "qemu invocation should include -netdev")
	assertContains(t, logStr, "virtio-net-pci", "qemu invocation should include virtio-net-pci")
	// Every instance must appear via -name flag.
	for _, name := range []string{"db-0", "web-0", "web-1"} {
		assertContains(t, logStr, name, "qemu log should reference "+name)
	}

	// Stop web only.
	stopOut, _ := h.mustRun("stop", "-f", dir+"/holos.yaml", "web")
	assertContains(t, stopOut, "service: web", "stop should report web")

	projects = psList(t, h)
	proj = findProject(t, projects, "lifecycle")
	for _, svc := range proj.Services {
		for _, inst := range svc.Instances {
			if svc.Name == "web" && inst.Status != "stopped" {
				t.Fatalf("expected web/%s stopped; got %q", inst.Name, inst.Status)
			}
			if svc.Name == "db" && inst.Status != "running" {
				t.Fatalf("expected db/%s running; got %q", inst.Name, inst.Status)
			}
		}
	}

	// Restart the whole project via `start` (no service arg).
	startOut, _ := h.mustRun("start", "-f", dir+"/holos.yaml")
	assertContains(t, startOut, "2/2 running", "start should bring web back to 2/2")

	projects = psList(t, h)
	proj = findProject(t, projects, "lifecycle")
	for _, svc := range proj.Services {
		for _, inst := range svc.Instances {
			if inst.Status != "running" {
				t.Fatalf("expected %s running after start; got %q", inst.Name, inst.Status)
			}
		}
	}

	// Tear down and confirm state file is gone.
	if _, err := os.Stat(filepath.Join(h.stateDir, "projects", "lifecycle.json")); err != nil {
		t.Fatalf("project state file missing before down: %v", err)
	}
	downOut, _ := h.mustRun("down", "-f", dir+"/holos.yaml")
	assertContains(t, downOut, `project "lifecycle" removed`, "down confirmation")

	if _, err := os.Stat(filepath.Join(h.stateDir, "projects", "lifecycle.json")); !os.IsNotExist(err) {
		t.Fatalf("expected project state file removed; stat err=%v", err)
	}

	projects = psList(t, h)
	for _, p := range projects {
		if p.Name == "lifecycle" {
			t.Fatalf("lifecycle still listed after down: %+v", p)
		}
	}
}

// TestLifecycle_Idempotent_Up checks that a second `up` against an unchanged
// spec does not double-start or leak state.
func TestLifecycle_Idempotent_Up(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("idem", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: idem
services:
  solo:
    image: %s
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")
	firstPID := psList(t, h)[0].Services[0].Instances[0].PID
	firstHash := psList(t, h)[0].SpecHash

	h.mustRun("up", "-f", dir+"/holos.yaml")
	again := psList(t, h)[0]
	secondPID := again.Services[0].Instances[0].PID
	secondHash := again.SpecHash

	if firstHash != secondHash {
		t.Fatalf("spec hash changed between identical ups: %s -> %s", firstHash, secondHash)
	}
	if firstPID != secondPID {
		t.Fatalf("expected running PID to be preserved across idempotent up: %d -> %d", firstPID, secondPID)
	}
}

// TestLifecycle_SpecChangeRestarts verifies that mutating the compose file
// triggers a teardown-and-recreate when we `up` again.
func TestLifecycle_SpecChangeRestarts(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("mutate", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: mutate
services:
  svc:
    image: %s
    vm:
      memory_mb: 256
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")
	before := psList(t, h)[0]
	beforePID := before.Services[0].Instances[0].PID

	changed := strings.Replace(compose, "memory_mb: 256", "memory_mb: 512", 1)
	_, _ = writeFile(dir, "holos.yaml", changed)

	h.mustRun("up", "-f", dir+"/holos.yaml")
	after := psList(t, h)[0]
	afterPID := after.Services[0].Instances[0].PID

	if before.SpecHash == after.SpecHash {
		t.Fatalf("expected spec hash to change after memory_mb edit")
	}
	if beforePID == afterPID {
		t.Fatalf("expected PID to change after spec rewrite; got %d both times", beforePID)
	}
}

// TestLifecycle_PortConflict ensures the runtime rejects a static port
// already bound by another listener on the host.
func TestLifecycle_PortConflict(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("portconflict", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	// Hold a TCP port so the runtime's availability probe sees it taken.
	port := reserveLocalPort(t)
	compose := fmt.Sprintf(`
name: portconflict
services:
  web:
    image: %s
    ports:
      - "%d:80"
`, img, port)
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("up", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected up to fail when host port is taken")
	}
	assertContains(t, stderr, "unavailable", "port conflict error")
}

// TestLifecycle_UnknownService errors out cleanly when `start <svc>` names
// a service that does not exist.
func TestLifecycle_UnknownService(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("unknownsvc", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf("name: unknownsvc\nservices:\n  real:\n    image: %s\n", img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("start", "-f", dir+"/holos.yaml", "ghost")
	if err == nil {
		t.Fatal("expected start on unknown service to fail")
	}
	assertContains(t, stderr, "ghost", "unknown service error mentions name")
}
