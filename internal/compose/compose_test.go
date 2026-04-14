package compose

import (
	"os"
	"path/filepath"
	"testing"
)

const testCompose = `
name: testapp

services:
  db:
    image: ./base.qcow2
    vm:
      vcpu: 2
      memory_mb: 1024
    cloud_init:
      packages:
        - postgresql

  api:
    image: ./base.qcow2
    depends_on:
      - db
    ports:
      - "3000:3000"
    cloud_init:
      packages:
        - nodejs

  web:
    image: ./base.qcow2
    replicas: 2
    depends_on:
      - api
    ports:
      - "8080:80"
    volumes:
      - ./www:/srv/www:ro
`

func TestLoadAndResolve(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composePath := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(composePath, []byte(testCompose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "www"), 0o755); err != nil {
		t.Fatal(err)
	}

	file, err := Load(composePath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if file.Name != "testapp" {
		t.Fatalf("expected name testapp, got %s", file.Name)
	}
	if len(file.Services) != 3 {
		t.Fatalf("expected 3 services, got %d", len(file.Services))
	}

	stateDir := filepath.Join(dir, "state")
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if project.Name != "testapp" {
		t.Fatalf("expected project name testapp, got %s", project.Name)
	}
	if project.SpecHash == "" {
		t.Fatal("expected non-empty spec hash")
	}

	// db has no dependencies, should come first.
	if project.ServiceOrder[0] != "db" {
		t.Fatalf("expected db first in order, got %v", project.ServiceOrder)
	}

	// web depends on api which depends on db, so web must be last.
	if project.ServiceOrder[len(project.ServiceOrder)-1] != "web" {
		t.Fatalf("expected web last in order, got %v", project.ServiceOrder)
	}

	web := project.Services["web"]
	if web.Replicas != 2 {
		t.Fatalf("expected web replicas 2, got %d", web.Replicas)
	}
	if len(web.Ports) != 1 || web.Ports[0].HostPort != 8080 || web.Ports[0].GuestPort != 80 {
		t.Fatalf("unexpected web ports: %+v", web.Ports)
	}
	if len(web.Mounts) != 1 || web.Mounts[0].Target != "/srv/www" || !web.Mounts[0].ReadOnly {
		t.Fatalf("unexpected web mounts: %+v", web.Mounts)
	}
	if web.InternalNetwork == nil {
		t.Fatal("expected internal network config on web service")
	}
	if len(web.InternalNetwork.InstanceIPs) != 2 {
		t.Fatalf("expected 2 instance IPs for web, got %d", len(web.InternalNetwork.InstanceIPs))
	}

	if len(project.Network.Hosts) == 0 {
		t.Fatal("expected hosts map to be populated")
	}
	if _, ok := project.Network.Hosts["db"]; !ok {
		t.Fatal("expected db in hosts")
	}
	if _, ok := project.Network.Hosts["web"]; !ok {
		t.Fatal("expected web in hosts")
	}
}

func TestTopoSortDetectsCycle(t *testing.T) {
	t.Parallel()

	file := &File{
		Name: "cycle",
		Services: map[string]Service{
			"a": {Image: "x.qcow2", DependsOn: []string{"b"}},
			"b": {Image: "x.qcow2", DependsOn: []string{"a"}},
		},
	}

	_, err := file.topoSort()
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec     string
		host     int
		guest    int
		protocol string
	}{
		{"8080:80", 8080, 80, "tcp"},
		{"443:443/tcp", 443, 443, "tcp"},
		{"80", 0, 80, "tcp"},
	}

	for _, tt := range tests {
		pf, err := parsePort(tt.spec)
		if err != nil {
			t.Fatalf("parsePort(%q): %v", tt.spec, err)
		}
		if pf.HostPort != tt.host || pf.GuestPort != tt.guest || pf.Protocol != tt.protocol {
			t.Fatalf("parsePort(%q) = %+v, want host=%d guest=%d proto=%s", tt.spec, pf, tt.host, tt.guest, tt.protocol)
		}
	}
}

func TestParseVolume(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	mount, err := parseVolume("./data:/var/lib/db:ro", dir)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if !filepath.IsAbs(mount.Source) {
		t.Fatalf("expected absolute source, got %s", mount.Source)
	}
	if mount.Target != "/var/lib/db" {
		t.Fatalf("expected target /var/lib/db, got %s", mount.Target)
	}
	if !mount.ReadOnly {
		t.Fatal("expected read-only mount")
	}
}

func TestFindFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composePath := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(composePath, []byte("name: test\nservices:\n  x:\n    image: a.qcow2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := FindFile(dir)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != composePath {
		t.Fatalf("expected %s, got %s", composePath, found)
	}
}

func TestValidateRejectsEmptyServices(t *testing.T) {
	t.Parallel()

	file := &File{Name: "test", Services: map[string]Service{}}
	if err := file.validate(); err == nil {
		t.Fatal("expected validation error for empty services")
	}
}

func TestValidateRejectsMissingDependency(t *testing.T) {
	t.Parallel()

	file := &File{
		Name: "test",
		Services: map[string]Service{
			"a": {Image: "x.qcow2", DependsOn: []string{"nonexistent"}},
		},
	}
	if err := file.validate(); err == nil {
		t.Fatal("expected validation error for missing dependency")
	}
}
