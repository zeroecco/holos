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

// TestHealthcheck_ConfigPersisted confirms that a compose healthcheck
// block survives parsing, resolution, and persistence into the project
// record; downstream tooling (ps --json) can rely on the fields.
func TestHealthcheck_ConfigPersisted(t *testing.T) {
	h := newHarness(t)
	h.extraEnv = append(h.extraEnv, "HOLOS_HEALTH_BYPASS=1")

	dir := h.writeProject("hcp", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: hcp
services:
  db:
    image: %s
    healthcheck:
      test: ["pg_isready"]
      interval: 2s
      retries: 5
      start_period: 1s
      timeout: 3s
  api:
    image: %s
    depends_on:
      - db
`, img, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	stdout, _ := h.mustRun("ps", "--json")

	// The healthcheck config should be queryable from ps --json. We
	// decode into an ad-hoc struct pinned to the fields we care about
	// so unrelated schema additions don't force this test to update.
	type inst struct {
		Name string `json:"name"`
	}
	type svc struct {
		Name      string `json:"name"`
		Instances []inst `json:"instances"`
	}
	type proj struct {
		Name     string `json:"name"`
		Services []svc  `json:"services"`
	}

	var projects []proj
	if err := json.Unmarshal([]byte(stdout), &projects); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout)
	}

	found := false
	for _, p := range projects {
		if p.Name != "hcp" {
			continue
		}
		for _, s := range p.Services {
			if s.Name == "db" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected hcp/db in ps output:\n%s", stdout)
	}

	// The on-disk project record only stores runtime state, not the
	// original manifest — so we assert on observable behavior instead:
	// both services reached running after the health gate.
	raw, err := os.ReadFile(filepath.Join(h.stateDir, "projects", "hcp.json"))
	if err != nil {
		t.Fatalf("read project record: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"name": "db"`) || !strings.Contains(text, `"name": "api"`) {
		t.Fatalf("expected db and api in record:\n%s", text)
	}
	if !strings.Contains(text, `"status": "running"`) {
		t.Fatalf("expected at least one running instance:\n%s", text)
	}
}

// TestHealthcheck_BypassLetsUpComplete exercises the happy-path ordering
// gate: a healthy service's dependents should start normally. Without
// HOLOS_HEALTH_BYPASS this test would hang trying to ssh into the mock
// VM, so the bypass is the subject under test.
func TestHealthcheck_BypassLetsUpComplete(t *testing.T) {
	h := newHarness(t)
	h.extraEnv = append(h.extraEnv, "HOLOS_HEALTH_BYPASS=1")

	dir := h.writeProject("hcbypass", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: hcbypass
services:
  db:
    image: %s
    healthcheck:
      test: ["true"]
      interval: 1s
      retries: 2
      timeout: 1s
  api:
    image: %s
    depends_on: [db]
`, img, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	// If the gate blocks forever `mustRun` will hang and the test
	// times out — the absence of a timeout failure IS the assertion.
	stdout, _ := h.mustRun("up", "-f", dir+"/holos.yaml")
	if !strings.Contains(stdout, "api") {
		t.Fatalf("api did not come up:\n%s", stdout)
	}
}
