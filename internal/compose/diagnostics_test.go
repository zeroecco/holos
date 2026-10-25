package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibilityWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "holos.yaml")
	source := `name: demo
services:
  web:
    image: ./base.qcow2
    restart: always
    profiles: [dev]
    read_only: false
    build:
      context: .
      args: {TOKEN: secret-value}
    deploy:
      resources:
        limits: {cpus: '2', memory: 1G, pids: 0}
    depends_on:
      db: {condition: service_completed_successfully}
    networks:
      default: {aliases: [www], mac_address: '02:00:00:00:00:01'}
    volumes:
      - {type: tmpfs, target: /tmp}
    x-note: ignored extension
  db: {image: ./base.qcow2}
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	warnings := f.CompatibilityWarnings()
	got := strings.Join(warnings, "\n")
	for _, field := range []string{"restart", "profiles", "read_only", "build.args", "deploy.resources.limits.pids", "depends_on.db.condition", "volumes[0]"} {
		if !strings.Contains(got, "services.web."+field+":") {
			t.Errorf("missing %s in %s", field, got)
		}
	}
	if len(warnings) != 7 || strings.Contains(got, "secret-value") {
		t.Fatalf("unexpected diagnostics: %s", got)
	}
}

func TestCompatibilityWarningsIncludesRespectOverrides(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"included.yaml": "services:\n  web: {image: ./base.qcow2, restart: always}\n  db: {image: ./base.qcow2, read_only: false}\n",
		"holos.yaml":    "name: demo\ninclude: [included.yaml]\nservices:\n  web: {image: ./base.qcow2}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := Load(filepath.Join(dir, "holos.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := f.CompatibilityWarnings()
	if len(got) != 1 || !strings.HasPrefix(got[0], "services.db.read_only:") {
		t.Fatalf("warnings: %v", got)
	}
}
