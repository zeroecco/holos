//go:build integration

package integration

import (
	"bytes"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// reserveLocalPort opens a TCP listener on 127.0.0.1, registers a cleanup
// to close it, and returns the port number. Any attempt to bind the same
// port from a different socket will fail until the test ends.
func reserveLocalPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type")
	}
	return addr.Port
}

func TestSimultaneousUpRetriesEphemeralPortRace(t *testing.T) {
	h := newHarness(t)
	h.extraEnv = append(h.extraEnv,
		"HOLOS_MOCK_BIND_HOSTFWD=1",
		"HOLOS_TEST_EPHEMERAL_PORTS=24000,24001,24002,24003,24004,24005,24006,24007",
	)

	projectA := h.writeProject("race-a", `
name: race-a
services:
  web:
    image: ./base.qcow2
    ports:
      - "80"
`, nil)
	h.fakeImage(projectA, "base.qcow2")

	projectB := h.writeProject("race-b", `
name: race-b
services:
  web:
    image: ./base.qcow2
    ports:
      - "80"
`, nil)
	h.fakeImage(projectB, "base.qcow2")

	type result struct {
		stdout string
		stderr string
		err    error
	}
	runUp := func(cwd string) result {
		cmd := exec.Command(h.binary, "up")
		cmd.Dir = cwd
		cmd.Env = h.env()
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return result{stdout: stdout.String(), stderr: stderr.String(), err: err}
	}

	var wg sync.WaitGroup
	results := make(chan result, 2)
	for _, dir := range []string{projectA, projectB} {
		dir := dir
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runUp(dir)
		}()
	}
	wg.Wait()
	close(results)

	var combinedStderr string
	for res := range results {
		if res.err != nil {
			t.Fatalf("concurrent holos up failed: %v\nstdout:\n%s\nstderr:\n%s", res.err, res.stdout, res.stderr)
		}
		combinedStderr += res.stderr
	}
	if !strings.Contains(combinedStderr, "host port conflict") {
		t.Fatalf("expected one up to report a host port retry, stderr:\n%s", combinedStderr)
	}

	h.mustRun("down", "race-a")
	h.mustRun("down", "race-b")
}
