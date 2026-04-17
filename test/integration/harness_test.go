//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// harness is a per-test environment with a compiled holos binary, a
// private state directory, and a PATH populated with mock qemu tools.
type harness struct {
	t        *testing.T
	binary   string
	stateDir string
	mockBin  string
	workDir  string
	qemuLog  string
	extraEnv []string
}

var (
	buildOnce  sync.Once
	builtPaths struct {
		holos string
		bins  string
		err   error
	}
)

// buildArtifacts compiles the holos CLI and the three mock binaries once
// per `go test` process and returns their paths.
func buildArtifacts(t *testing.T) (holosBin string, mockBinDir string) {
	t.Helper()

	buildOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			builtPaths.err = err
			return
		}

		// We store compiled artifacts under a stable tmp dir shared across
		// tests in the same `go test` invocation. TempDir on the process
		// directly (os.MkdirTemp) is cleaned up via t.Cleanup from a
		// sentinel test, but here we rely on OS tmp cleanup to avoid
		// coupling to any one test's lifetime.
		artifactDir, err := os.MkdirTemp("", "holos-itest-artifacts-")
		if err != nil {
			builtPaths.err = err
			return
		}

		holosOut := filepath.Join(artifactDir, "holos")
		if err := goBuild(root, filepath.Join(root, "cmd/holos"), holosOut); err != nil {
			builtPaths.err = fmt.Errorf("build holos: %w", err)
			return
		}

		mockDir := filepath.Join(artifactDir, "bin")
		if err := os.MkdirAll(mockDir, 0o755); err != nil {
			builtPaths.err = err
			return
		}
		mocks := []struct{ src, out string }{
			{filepath.Join(root, "test/integration/mocks/qemu-system"), filepath.Join(mockDir, "qemu-system-x86_64")},
			{filepath.Join(root, "test/integration/mocks/qemu-img"), filepath.Join(mockDir, "qemu-img")},
			{filepath.Join(root, "test/integration/mocks/cloud-localds"), filepath.Join(mockDir, "cloud-localds")},
		}
		for _, m := range mocks {
			if err := goBuild(root, m.src, m.out); err != nil {
				builtPaths.err = fmt.Errorf("build %s: %w", filepath.Base(m.out), err)
				return
			}
		}
		builtPaths.holos = holosOut
		builtPaths.bins = mockDir
	})

	if builtPaths.err != nil {
		t.Fatalf("build artifacts: %v", builtPaths.err)
	}
	return builtPaths.holos, builtPaths.bins
}

func goBuild(cwd, pkg, out string) error {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\n%s", pkg, err, stderr.String())
	}
	return nil
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	holos, mockDir := buildArtifacts(t)

	workDir := t.TempDir()
	stateDir := filepath.Join(workDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	return &harness{
		t:        t,
		binary:   holos,
		stateDir: stateDir,
		mockBin:  mockDir,
		workDir:  workDir,
		qemuLog:  filepath.Join(workDir, "qemu.log"),
	}
}

// env returns the environment variables passed to CLI invocations. We put
// the mock binaries first on PATH so the runtime picks them up.
func (h *harness) env() []string {
	pathValue := h.mockBin + string(os.PathListSeparator) + os.Getenv("PATH")
	env := []string{
		"PATH=" + pathValue,
		"HOME=" + h.workDir,
		"HOLOS_STATE_DIR=" + h.stateDir,
		"HOLOS_QEMU_SYSTEM=" + filepath.Join(h.mockBin, "qemu-system-x86_64"),
		"HOLOS_QEMU_IMG=" + filepath.Join(h.mockBin, "qemu-img"),
		"HOLOS_MOCK_QEMU_LOG=" + h.qemuLog,
	}
	env = append(env, h.extraEnv...)
	return env
}

// run invokes the holos binary with the given args and returns stdout,
// stderr, and any exec error. It never calls t.Fatal so tests can assert
// on expected failures.
func (h *harness) run(args ...string) (stdout, stderr string, err error) {
	return h.runIn(h.workDir, args...)
}

func (h *harness) runIn(cwd string, args ...string) (stdout, stderr string, err error) {
	h.t.Helper()

	cmd := exec.Command(h.binary, args...)
	cmd.Dir = cwd
	cmd.Env = h.env()

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// mustRun fails the test if the command exits non-zero.
func (h *harness) mustRun(args ...string) (stdout, stderr string) {
	h.t.Helper()
	stdout, stderr, err := h.run(args...)
	if err != nil {
		h.t.Fatalf("holos %s failed: %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr
}

// writeProject writes a compose file plus any extra fixture files into a
// fresh tmp dir and returns the project directory path.
func (h *harness) writeProject(name string, composeYAML string, extras map[string]string) string {
	h.t.Helper()

	dir := filepath.Join(h.workDir, "projects", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatalf("mkdir project dir: %v", err)
	}

	composePath := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(composePath, []byte(composeYAML), 0o644); err != nil {
		h.t.Fatalf("write compose: %v", err)
	}

	for rel, content := range extras {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			h.t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			h.t.Fatalf("write %s: %v", path, err)
		}
	}

	return dir
}

// fakeImage writes a dummy qcow2-looking file at path inside dir and returns
// the relative path usable as `image:` in a compose file.
func (h *harness) fakeImage(dir, name string) string {
	h.t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fake-qcow2-header"), 0o644); err != nil {
		h.t.Fatalf("write fake image: %v", err)
	}
	return "./" + name
}

// assertContains fails the test if substr is not found in haystack.
func assertContains(t *testing.T, haystack, substr, context string) {
	t.Helper()
	if !strings.Contains(haystack, substr) {
		t.Fatalf("%s: expected to contain %q; got:\n%s", context, substr, haystack)
	}
}

func assertNotContains(t *testing.T, haystack, substr, context string) {
	t.Helper()
	if strings.Contains(haystack, substr) {
		t.Fatalf("%s: expected NOT to contain %q; got:\n%s", context, substr, haystack)
	}
}

// repoRoot walks up from the test file's directory looking for go.mod.
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate test source file")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", filepath.Dir(thisFile))
		}
		dir = parent
	}
}
