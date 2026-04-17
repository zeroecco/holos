//go:build integration

package integration

import (
	"strings"
	"testing"
)

// TestCLI_Help confirms the top-level binary prints usage on -h/help and
// exits non-zero when no command is supplied.
func TestCLI_Help(t *testing.T) {
	h := newHarness(t)

	for _, arg := range []string{"-h", "--help", "help"} {
		stdout, stderr, err := h.run(arg)
		if err != nil {
			t.Fatalf("help via %q failed: %v\nstdout:%s\nstderr:%s", arg, err, stdout, stderr)
		}
		assertContains(t, stderr, "holos - docker compose for KVM",
			"help banner ("+arg+")")
		for _, cmd := range []string{"up", "down", "ps", "validate", "pull", "images", "devices"} {
			assertContains(t, stderr, "holos "+cmd,
				"help should list "+cmd)
		}
	}

	_, stderr, err := h.run()
	if err == nil {
		t.Fatal("expected non-zero exit for bare invocation")
	}
	assertContains(t, stderr, "missing command", "bare invocation stderr")
}

func TestCLI_UnknownCommand(t *testing.T) {
	h := newHarness(t)

	_, stderr, err := h.run("not-a-command")
	if err == nil {
		t.Fatal("expected non-zero exit for unknown command")
	}
	assertContains(t, stderr, "unknown command", "unknown command stderr")
}

// TestCLI_Images verifies the built-in image registry is reachable via the
// CLI without requiring network access.
func TestCLI_Images(t *testing.T) {
	h := newHarness(t)

	stdout, _ := h.mustRun("images")
	for _, name := range []string{"alpine", "arch", "debian", "ubuntu", "fedora"} {
		assertContains(t, stdout, name, "images output should list "+name)
	}
	assertContains(t, stdout, "NAME", "images output should have header")
	assertContains(t, stdout, "TAG", "images output should have header")
}

// TestCLI_PS_Empty shows that ps on a fresh state dir is well-behaved.
func TestCLI_PS_Empty(t *testing.T) {
	h := newHarness(t)

	stdout, _ := h.mustRun("ps")
	assertContains(t, stdout, "no running projects", "ps with empty state")
}

func TestCLI_PS_JSON_Empty(t *testing.T) {
	h := newHarness(t)

	stdout, _ := h.mustRun("ps", "-json")
	trimmed := strings.TrimSpace(stdout)
	if trimmed != "[]" {
		t.Fatalf("expected [] from ps -json; got %q", trimmed)
	}
}

func TestCLI_Validate_NoFile(t *testing.T) {
	h := newHarness(t)

	emptyDir := h.writeProject("empty", "name: anything\nservices: {}\n", nil)
	// Shadow by running in a dir without holos.yaml (a child directory).
	inner := emptyDir + "/no-compose-here"
	if _, stderr, err := h.runIn(emptyDir, "validate", "-f", inner); err == nil {
		t.Fatalf("expected validate to fail for missing file; stderr:%s", stderr)
	}
}

func TestCLI_Pull_LocalPathPassthrough(t *testing.T) {
	h := newHarness(t)

	// Local paths must not trigger a network fetch: Pull should echo the
	// reference back and infer a format from the extension.
	stdout, _ := h.mustRun("pull", "./no-such-image.qcow2")
	assertContains(t, stdout, "image:", "pull should print image:")
	assertContains(t, stdout, "format: qcow2", "pull should infer qcow2 format")
}

func TestCLI_Pull_UnknownImage(t *testing.T) {
	h := newHarness(t)

	_, stderr, err := h.run("pull", "not-a-real-distro")
	if err == nil {
		t.Fatal("expected pull to fail for unknown image")
	}
	assertContains(t, stderr, "unknown image", "pull unknown image error")
}
