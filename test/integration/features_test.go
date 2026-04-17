//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeature_VolumesAndMounts confirms volume mounts are passed through to
// qemu as -virtfs arguments.
func TestFeature_VolumesAndMounts(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("mounts", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		t.Fatal(err)
	}

	compose := fmt.Sprintf(`
name: mounts
services:
  web:
    image: %s
    volumes:
      - ./www:/srv/www:ro
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	logData, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu log: %v", err)
	}
	logStr := string(logData)
	assertContains(t, logStr, "-virtfs", "virtfs arg missing")
	assertContains(t, logStr, "readonly=on", "virtfs should be readonly")
	assertContains(t, logStr, "/www", "virtfs path should reference ./www")
}

// TestFeature_ExtraArgs verifies that user-supplied vm.extra_args appear
// verbatim on the qemu command line.
func TestFeature_ExtraArgs(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("extra", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: extra
services:
  x:
    image: %s
    vm:
      extra_args:
        - "-object"
        - "memory-backend-file,id=mb1,size=256M,mem-path=/tmp/holos-test"
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	logData, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu log: %v", err)
	}
	logStr := string(logData)
	assertContains(t, logStr, "-object", "extra args should include -object")
	assertContains(t, logStr, "memory-backend-file", "extra args should include backing")
}

// TestFeature_UEFIAutoEnableWithDevices confirms that declaring a device
// automatically enables UEFI.
func TestFeature_UEFIAutoEnableWithDevices(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("uefi", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: uefi
services:
  gpu:
    image: %s
    devices:
      - pci: "0000:01:00.0"
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	// UEFI requires OVMF files; the mock qemu doesn't care but the
	// runtime will look for OVMF_CODE.fd before it even launches qemu.
	// Setting HOLOS_OVMF_CODE to any readable file is enough for the
	// find routine to short-circuit. We also need OVMF_VARS.
	ovmfCode := filepath.Join(dir, "OVMF_CODE.fd")
	ovmfVars := filepath.Join(dir, "OVMF_VARS.fd")
	for _, p := range []string{ovmfCode, ovmfVars} {
		if err := os.WriteFile(p, []byte("mock-ovmf"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.extraEnv = append(h.extraEnv,
		"HOLOS_OVMF_CODE="+ovmfCode,
		"HOLOS_OVMF_VARS="+ovmfVars,
	)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	logData, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu log: %v", err)
	}
	logStr := string(logData)
	assertContains(t, logStr, "pflash", "UEFI requires pflash drives")
	assertContains(t, logStr, "vfio-pci", "VFIO device should be passed to qemu")
	assertContains(t, logStr, "kernel-irqchip=on", "machine should enable kernel-irqchip for VFIO")
}

// TestFeature_InternalNetworking checks that the socket multicast netdev is
// present for multi-service projects and that the multicast group/port are
// stable between invocations.
func TestFeature_InternalNetworking(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("net", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: net
services:
  api:
    image: %s
  worker:
    image: %s
    depends_on: [api]
`, img, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	logData, err := os.ReadFile(h.qemuLog)
	if err != nil {
		t.Fatalf("read qemu log: %v", err)
	}
	logStr := string(logData)
	assertContains(t, logStr, "socket,id=net1,mcast=", "socket mcast netdev missing")
	// Multicast group for RFC2365 range.
	if !strings.Contains(logStr, "mcast=239.") {
		limit := len(logStr)
		if limit > 400 {
			limit = 400
		}
		t.Fatalf("expected multicast group in 239.0.0.0/8; log:\n%s", logStr[:limit])
	}

	projects := psList(t, h)
	proj := findProject(t, projects, "net")
	if proj.Network.MulticastGroup == "" || proj.Network.MulticastPort == 0 {
		t.Fatalf("expected network plan in ps output; got %+v", proj.Network)
	}
}

// TestFeature_Logs_ServiceNotFound provides a cheap end-to-end assertion of
// the `holos logs` error path (real log tailing requires a booted VM).
func TestFeature_Logs_ServiceNotFound(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("logs", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf("name: logs\nservices:\n  svc:\n    image: %s\n", img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	_, stderr, err := h.run("logs", "-f", dir+"/holos.yaml", "missing")
	if err == nil {
		t.Fatal("expected logs on unknown service to fail")
	}
	assertContains(t, stderr, "missing", "logs error should mention missing service")
}

// TestFeature_Logs_Tail writes to the QEMU console log and confirms the
// logs command echoes it back.
func TestFeature_Logs_Tail(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("logtail", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf("name: logtail\nservices:\n  svc:\n    image: %s\n", img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	h.mustRun("up", "-f", dir+"/holos.yaml")

	projects := psList(t, h)
	proj := findProject(t, projects, "logtail")
	if len(proj.Services) == 0 || len(proj.Services[0].Instances) == 0 {
		t.Fatal("expected at least one instance")
	}
	inst := proj.Services[0].Instances[0]
	logPath := filepath.Join(inst.WorkDir, "console.log")
	if err := os.WriteFile(logPath, []byte("line-one\nline-two\nline-three\n"), 0o644); err != nil {
		t.Fatalf("seed console log: %v", err)
	}

	stdout, _ := h.mustRun("logs", "-f", dir+"/holos.yaml", "svc")
	for _, want := range []string{"line-one", "line-two", "line-three", "svc-0"} {
		assertContains(t, stdout, want, "logs output should include "+want)
	}
}
