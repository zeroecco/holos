//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
)

// TestValidate_SingleService exercises the validate command on a minimal
// compose file and confirms the summary contains service/network info.
func TestValidate_SingleService(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("one", "", nil)
	imageRel := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: one
services:
  web:
    image: %s
    replicas: 1
    ports:
      - "8080:80"
`, imageRel)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	stdout, _ := h.mustRun("validate", "-f", dir+"/holos.yaml")
	for _, want := range []string{
		"project: one",
		"spec_hash:",
		"services: 1",
		"web",
		"network:",
		"10.10.0.0/24",
		"239.",
	} {
		assertContains(t, stdout, want, "validate output")
	}
}

// TestValidate_MultiServiceWithDeps confirms topological ordering appears
// in the validate summary.
func TestValidate_MultiServiceWithDeps(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("stack", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: stack
services:
  web:
    image: %s
    replicas: 2
    depends_on: [api]
    ports:
      - "8080:80"
  api:
    image: %s
    depends_on: [db]
    ports:
      - "3000:3000"
  db:
    image: %s
    vm:
      vcpu: 2
      memory_mb: 1024
`, img, img, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	stdout, _ := h.mustRun("validate", "-f", dir+"/holos.yaml")
	assertContains(t, stdout, "services: 3", "service count")
	assertContains(t, stdout, "order: [db api web]", "topological order")

	// Each replica of web must appear in the hosts block (web-0, web-1).
	for _, host := range []string{"db", "api", "web", "web-0", "web-1"} {
		assertContains(t, stdout, host, "hosts block should list "+host)
	}
}

func TestValidate_CircularDependency(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("cycle", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: cycle
services:
  a:
    image: %s
    depends_on: [b]
  b:
    image: %s
    depends_on: [a]
`, img, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to fail on cycle")
	}
	assertContains(t, stderr, "circular", "cycle error")
}

func TestValidate_RejectsInvalidNames(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("bad-names", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: BadName
services:
  web:
    image: %s
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to reject uppercase project name")
	}
	assertContains(t, stderr, "project name", "invalid project name error")
}

func TestValidate_RejectsMissingImage(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("no-image", "", nil)
	compose := `
name: no-image
services:
  web:
    replicas: 1
`
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to reject service without image")
	}
	assertContains(t, stderr, "image", "missing image error")
}

func TestValidate_RejectsUDPPort(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("udp", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: udp
services:
  dns:
    image: %s
    ports:
      - "53:53/udp"
`, img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to reject UDP port")
	}
	assertContains(t, stderr, "tcp", "udp rejection error")
}

// TestValidate_UsesCWDCompose covers the implicit holos.yaml discovery.
func TestValidate_UsesCWDCompose(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("implicit", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf("name: implicit\nservices:\n  x:\n    image: %s\n", img)
	_, _ = writeFile(dir, "holos.yaml", compose)

	stdout, _, err := h.runIn(dir, "validate")
	if err != nil {
		t.Fatalf("validate in cwd: %v", err)
	}
	assertContains(t, stdout, "project: implicit", "validate cwd output")
}

// TestValidate_AllExamples exercises runnable compose files shipped under
// examples/ except the netboot sample (Dockerfile-backed) and gpu-passthrough
// (host-specific devices).
func TestValidate_AllExamples(t *testing.T) {
	h := newHarness(t)
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}

	for _, tc := range []struct {
		name     string
		path     string
		imageRef string
	}{
		{name: "alpine-nginx", path: "examples/alpine-nginx/holos.yaml", imageRef: "alpine"},
		{name: "minecraft-server", path: "examples/minecraft-server/holos.yaml", imageRef: "ubuntu:noble"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Registry images would trigger network downloads during
			// validation. Copy the example into a tmp dir and swap in a
			// local fake image so we cover the real YAML structure without
			// any HTTP.
			original := root + "/" + tc.path
			data, err := readFile(original)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			dir := h.writeProject(tc.name+"-copy", "", nil)
			img := h.fakeImage(dir, "base.qcow2")
			rewritten := strings.Replace(data, "image: "+tc.imageRef, "image: "+img, 1)
			if rewritten == data {
				t.Fatalf("example %s did not contain image ref %q", tc.path, tc.imageRef)
			}
			if _, err := writeFile(dir, "holos.yaml", rewritten); err != nil {
				t.Fatalf("write compose: %v", err)
			}

			stdout, _ := h.mustRun("validate", "-f", dir+"/holos.yaml")
			assertContains(t, stdout, "project: "+tc.name, "example validation")
			assertContains(t, stdout, "services: 1", "example service count")
		})
	}
}
