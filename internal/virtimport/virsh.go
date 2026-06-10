package virtimport

import (
	"fmt"
	"os/exec"
	"strings"
)

// Virsh is a thin wrapper around the virsh CLI used to fetch domain
// XML. Tests substitute their own implementation by passing fixture
// XML directly to Convert; this struct only matters at the CLI layer.
type Virsh struct {
	// Binary is the path to the virsh executable. Empty means look
	// up "virsh" on PATH.
	Binary string
	// URI is passed as `-c <uri>` to virsh. Empty means use the
	// libvirt default (qemu:///system for root, qemu:///session for
	// regular users).
	URI string
}

// DumpXML returns the raw XML for a single domain.
func (v Virsh) DumpXML(domain string) ([]byte, error) {
	out, err := v.run("dumpxml", domain)
	if err != nil {
		return nil, fmt.Errorf("virsh dumpxml %s: %w", domain, err)
	}
	return out, nil
}

// ListDomains returns the names of every defined domain (running or
// shut off). It uses --name so the output is one bare name per line
// without status columns to parse around.
func (v Virsh) ListDomains() ([]string, error) {
	out, err := v.run("list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("virsh list: %w", err)
	}
	return parseVirshDomainNames(out), nil
}

func (v Virsh) run(args ...string) ([]byte, error) {
	bin := v.Binary
	if bin == "" {
		bin = "virsh"
	}
	full := []string{}
	if v.URI != "" {
		full = append(full, "-c", v.URI)
	}
	full = append(full, args...)
	cmd := exec.Command(bin, full...)
	out, err := cmd.Output()
	if err != nil {
		if stderr, ok := exitErrorStderr(err); ok {
			return nil, fmt.Errorf("%s: %s", err, stderr)
		}
		return nil, err
	}
	return out, nil
}

func exitErrorStderr(err error) (string, bool) {
	exitErr, ok := err.(*exec.ExitError)
	if !ok || !exitErrorHasStderr(exitErr) {
		return "", false
	}
	return strings.TrimSpace(string(exitErr.Stderr)), true
}

func exitErrorHasStderr(err *exec.ExitError) bool {
	return len(err.Stderr) > 0
}
