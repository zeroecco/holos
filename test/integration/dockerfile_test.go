//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestDockerfile_EndToEnd exercises the Dockerfile code path: a service
// declared with dockerfile: (no image:) must pick up FROM from the
// Dockerfile and be bootable end-to-end through the CLI.
func TestDockerfile_EndToEnd(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("df", "", nil)
	img := h.fakeImage(dir, "from.qcow2")

	dockerfile := "FROM " + img + "\n" +
		"ENV FOO=bar\n" +
		"RUN echo hello > /tmp/hi\n" +
		"COPY site.conf /etc/nginx/conf.d/site.conf\n" +
		"WORKDIR /srv\n"

	if _, err := writeFile(dir, "Dockerfile", dockerfile); err != nil {
		t.Fatal(err)
	}
	if _, err := writeFile(dir, "site.conf", "server { listen 80; }\n"); err != nil {
		t.Fatal(err)
	}

	compose := `
name: df
services:
  app:
    dockerfile: ./Dockerfile
`
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	stdout, _ := h.mustRun("up", "-f", dir+"/holos.yaml")
	assertContains(t, stdout, "1/1 running", "dockerfile service should start")

	// Validate is a good surface check for the dockerfile FROM resolution.
	validateOut, _ := h.mustRun("validate", "-f", dir+"/holos.yaml")
	assertContains(t, validateOut, "from.qcow2", "FROM should resolve to local image path")
}

// TestDockerfile_MissingCopySource should surface a clean error.
func TestDockerfile_MissingCopySource(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("dfmiss", "", nil)
	img := h.fakeImage(dir, "from.qcow2")

	dockerfile := fmt.Sprintf("FROM %s\nCOPY missing.txt /opt/missing.txt\n", img)
	_, _ = writeFile(dir, "Dockerfile", dockerfile)
	compose := `
name: dfmiss
services:
  app:
    dockerfile: ./Dockerfile
`
	_, _ = writeFile(dir, "holos.yaml", compose)

	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to fail when COPY source is missing")
	}
	assertContains(t, stderr, "missing.txt", "error should mention missing file")
}
