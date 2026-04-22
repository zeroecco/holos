//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstall_DryRunEmitsUnit renders the unit to stdout without
// touching the filesystem. The fast feedback loop operators use to
// review what they're about to commit.
func TestInstall_DryRunEmitsUnit(t *testing.T) {
	h := newHarness(t)

	xdg := filepath.Join(h.workDir, "xdg")
	h.extraEnv = append(h.extraEnv, "XDG_CONFIG_HOME="+xdg)

	dir := h.writeProject("rebootproj", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: rebootproj
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	stdout, _ := h.mustRun("install", "-f", filepath.Join(dir, "holos.yaml"), "--dry-run")

	// Key fragments the unit must contain: binary-absolute ExecStart,
	// a project-scoped Description, and default.target for user scope.
	// The harness always passes --state-dir, so the unit must render
	// with --state-dir between the subcommand and the (compose path or
	// project name) positional. A trailing --state-dir would be silently
	// dropped at boot/shutdown; see TestRender_StateFlagBeforePositional
	// in internal/systemd for the unit-test pin of the same contract.
	for _, want := range []string{
		"ExecStart=",
		" up --state-dir " + h.stateDir + " -f " + filepath.Join(dir, "holos.yaml"),
		"ExecStop=",
		" down --state-dir " + h.stateDir + " rebootproj",
		"Description=holos project rebootproj",
		"WantedBy=default.target",
		"# would write to:",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, stdout)
		}
	}

	// Dry-run must NOT create any file on disk.
	unit := filepath.Join(xdg, "systemd", "user", "holos-rebootproj.service")
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("dry-run unexpectedly wrote unit file: %v", err)
	}
}

// TestInstall_WritesUserUnit exercises the real install path: the
// file should land under XDG_CONFIG_HOME/systemd/user and contain
// absolute paths resolved from the compose file.
func TestInstall_WritesUserUnit(t *testing.T) {
	h := newHarness(t)

	xdg := filepath.Join(h.workDir, "xdg")
	h.extraEnv = append(h.extraEnv, "XDG_CONFIG_HOME="+xdg)

	dir := h.writeProject("rebootproj2", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: rebootproj2
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	stdout, _ := h.mustRun("install", "-f", filepath.Join(dir, "holos.yaml"))
	unit := filepath.Join(xdg, "systemd", "user", "holos-rebootproj2.service")
	if !strings.Contains(stdout, unit) {
		t.Fatalf("install output missing %q:\n%s", unit, stdout)
	}
	content, err := os.ReadFile(unit)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	body := string(content)

	// Absolute compose path, absolute state-dir, and the binary path
	// that was actually used. Operators should never see a relative
	// path in a unit because at boot $PWD is /.
	if !strings.Contains(body, filepath.Join(dir, "holos.yaml")) {
		t.Fatalf("unit missing abs compose path:\n%s", body)
	}
	if !strings.Contains(body, "--state-dir "+h.stateDir) {
		t.Fatalf("unit missing state-dir flag:\n%s", body)
	}
	if !strings.Contains(body, h.binary) {
		t.Fatalf("unit missing holos binary path %q:\n%s", h.binary, body)
	}
}

// TestInstall_SystemUserRequiresExplicitStateDir pins the guardrail
// that prevents the silent-failure trap where `sudo holos install
// --system --user alice` emits a unit whose User= cannot read the
// default (root-owned, 0700) state directory.
func TestInstall_SystemUserRequiresExplicitStateDir(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("sysuser", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: sysuser
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	// The install CLI validator only trips when --state-dir was not
	// set on the command line. HOLOS_STATE_DIR (injected by the
	// harness) feeds the flag's default value but is invisible to
	// flag.Visit, so this still reproduces the "operator forgot to
	// think about where alice's state dir lives" trap the finding
	// flagged.
	cmd := []string{"install", "-f", filepath.Join(dir, "holos.yaml"), "--system", "--user", "alice", "--dry-run"}
	_, stderr, err := h.run(cmd...)
	if err == nil {
		t.Fatalf("expected error, got success; stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "--state-dir") || !strings.Contains(stderr, "alice") {
		t.Fatalf("error should steer the operator to --state-dir for alice, got: %s", stderr)
	}

	// With --state-dir supplied, the same command must succeed.
	cmd = append(cmd, "--state-dir", h.stateDir)
	if _, _, err := h.run(cmd...); err != nil {
		t.Fatalf("install with explicit --state-dir failed: %v", err)
	}
}

// TestUninstall_RemovesFile confirms the symmetry: install writes a
// file, uninstall removes it, and a second uninstall is a no-op so
// automation can retry safely.
func TestUninstall_RemovesFile(t *testing.T) {
	h := newHarness(t)

	xdg := filepath.Join(h.workDir, "xdg")
	h.extraEnv = append(h.extraEnv, "XDG_CONFIG_HOME="+xdg)

	dir := h.writeProject("rebootproj3", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: rebootproj3
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("install", "-f", filepath.Join(dir, "holos.yaml"))
	unit := filepath.Join(xdg, "systemd", "user", "holos-rebootproj3.service")
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("unit not installed: %v", err)
	}

	h.mustRun("uninstall", "-f", filepath.Join(dir, "holos.yaml"))
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("unit still present after uninstall: %v", err)
	}

	// Second uninstall on a removed unit must also succeed. This
	// keeps `holos uninstall` safe to script.
	h.mustRun("uninstall", "-f", filepath.Join(dir, "holos.yaml"))

	// With --name we don't even need the compose file.
	_, _ = h.mustRun("uninstall", "--name", "rebootproj3")
}
