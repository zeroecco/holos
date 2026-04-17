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

// TestExec_ProvisioningArtifacts verifies that `holos up` creates everything
// `holos exec` relies on: a project keypair, an ssh port forward in the
// persisted instance record, and a user-data authorized_keys entry.
func TestExec_ProvisioningArtifacts(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("execp", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: execp
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	// Keypair exists with restrictive perms on the private half.
	privPath := filepath.Join(h.stateDir, "ssh", "execp", "id_ed25519")
	pubPath := privPath + ".pub"
	if info, err := os.Stat(privPath); err != nil {
		t.Fatalf("missing private key: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %v, want 0600", info.Mode().Perm())
	}
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read pub key: %v", err)
	}
	pub := strings.TrimSpace(string(pubBytes))
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Fatalf("public key not ed25519-formatted:\n%s", pub)
	}

	// The generated key is injected into the instance's cloud-init
	// user-data so the guest accepts it on first boot.
	seedData, err := os.ReadFile(filepath.Join(h.stateDir, "instances", "execp", "web-0", "seed", "user-data"))
	if err != nil {
		t.Fatalf("read user-data: %v", err)
	}
	if !strings.Contains(string(seedData), pub) {
		t.Fatalf("user-data missing holos exec key:\n%s", string(seedData))
	}

	// ps --json exposes the allocated ssh_port for downstream tools.
	stdout, _ := h.mustRun("ps", "--json")
	var projects []struct {
		Name     string `json:"name"`
		Services []struct {
			Instances []struct {
				Name    string `json:"name"`
				SSHPort int    `json:"ssh_port"`
			} `json:"instances"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("decode ps json: %v\n%s", err, stdout)
	}
	var sshPort int
	for _, p := range projects {
		if p.Name != "execp" {
			continue
		}
		for _, s := range p.Services {
			for _, i := range s.Instances {
				if i.Name == "web-0" {
					sshPort = i.SSHPort
				}
			}
		}
	}
	if sshPort == 0 {
		t.Fatalf("expected non-zero ssh_port in ps --json:\n%s", stdout)
	}

	// QEMU must be told to forward sshPort → guest :22.
	logData, _ := os.ReadFile(h.qemuLog)
	hostfwd := fmt.Sprintf("hostfwd=tcp:127.0.0.1:%d-:22", sshPort)
	if !strings.Contains(string(logData), hostfwd) {
		t.Fatalf("qemu args missing %q:\n%s", hostfwd, string(logData))
	}
}

// TestExec_RejectsUnknownInstance makes sure the CLI fails with a useful
// message when the named instance doesn't exist, rather than silently
// trying to ssh to a dead port.
func TestExec_RejectsUnknownInstance(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("execp2", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: execp2
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}
	h.mustRun("up", "-f", dir+"/holos.yaml")

	_, stderr, err := h.run("exec", "-f", dir+"/holos.yaml", "ghost-0")
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
	if !strings.Contains(stderr, "ghost-0") {
		t.Fatalf("error should name the instance; got:\n%s", stderr)
	}
}

// TestExec_RejectsStoppedInstance ensures we don't attempt to ssh into a
// VM that's not currently running.
func TestExec_RejectsStoppedInstance(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("execp3", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: execp3
services:
  web:
    image: %s
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}
	h.mustRun("up", "-f", dir+"/holos.yaml")
	h.mustRun("stop", "-f", dir+"/holos.yaml")

	_, stderr, err := h.run("exec", "-f", dir+"/holos.yaml", "web-0")
	if err == nil {
		t.Fatal("expected error for stopped instance")
	}
	if !strings.Contains(stderr, "stopped") {
		t.Fatalf("error should mention stopped; got:\n%s", stderr)
	}
}
