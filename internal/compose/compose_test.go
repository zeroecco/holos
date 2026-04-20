package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/images"
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

func TestUserResolutionChain(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	// Pre-warm the image cache so resolve doesn't hit the network for
	// known distro refs. We only need each cached file to exist; its
	// contents don't matter for the user-resolution logic under test.
	cacheDir := filepath.Join(stateDir, "images")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"debian:bookworm", "alpine", "fedora"} {
		img, err := images.Resolve(ref)
		if err != nil || img == nil {
			t.Fatalf("pre-warm resolve(%q): img=%v err=%v", ref, img, err)
		}
		stub := filepath.Join(cacheDir, fmt.Sprintf("%s-%s-%s.qcow2",
			img.Name, img.Tag, sha256Prefix(img.URL)))
		if err := os.WriteFile(stub, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name        string
		image       string
		explicit    string
		wantUser    string
		description string
	}{
		{"explicit-wins", "debian:bookworm", "operator", "operator", "explicit cloud_init.user beats image default"},
		{"image-default-debian", "debian:bookworm", "", "debian", "debian image yields debian user"},
		{"image-default-alpine", "alpine", "", "alpine", "alpine image yields alpine user"},
		{"image-default-fedora", "fedora", "", "fedora", "fedora image yields fedora user"},
		{"local-falls-back", "./base.qcow2", "", "ubuntu", "local image falls back to ubuntu default"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := &File{
				Name: "usertest",
				Services: map[string]Service{
					"vm": {
						Image:     c.image,
						CloudInit: CloudInit{User: c.explicit},
					},
				},
			}
			project, err := file.Resolve(dir, stateDir)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := project.Services["vm"].CloudInit.User
			if got != c.wantUser {
				t.Errorf("%s: user = %q, want %q", c.description, got, c.wantUser)
			}
		})
	}
}

// sha256Prefix mirrors images.cacheFilename's URL-hash suffix without
// exporting it; tests only need the first 4 bytes (8 hex chars) of the
// URL's SHA-256 digest.
func sha256Prefix(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:4])
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

// parsePort previously accepted "/udp" and other non-TCP protocol suffixes,
// only for manifest validation to reject them later. The error must now
// surface at parse time.
func TestParsePortRejectsNonTCPProtocol(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{"53:53/udp", "80/sctp"} {
		if _, err := parsePort(spec); err == nil {
			t.Fatalf("parsePort(%q): expected error for non-tcp protocol", spec)
		}
	}
}

func TestParseVolume(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	mount, err := parseVolume("./data:/var/lib/db:ro", dir, nil)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.Kind != config.MountKindBind {
		t.Fatalf("expected bind kind, got %q", mount.Kind)
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

func TestParseVolume_Named(t *testing.T) {
	t.Parallel()

	declared := map[string]Volume{
		"data": {Size: "5G"},
	}

	mount, err := parseVolume("data:/var/lib/db", t.TempDir(), declared)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.Kind != config.MountKindVolume {
		t.Fatalf("expected volume kind, got %q", mount.Kind)
	}
	if mount.VolumeName != "data" {
		t.Fatalf("expected volume_name data, got %q", mount.VolumeName)
	}
	if got := int64(5) * (1 << 30); mount.SizeBytes != got {
		t.Fatalf("expected size %d bytes, got %d", got, mount.SizeBytes)
	}
	if mount.Source != "" {
		t.Fatalf("named volume should have no host source, got %q", mount.Source)
	}
}

// TestResolveHealthcheck_ListForm confirms the YAML `test:` list form
// flows through to the resolved config unchanged.
func TestResolveHealthcheck_ListForm(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hc
services:
  api:
    image: ./img.qcow2
    healthcheck:
      test: ["curl", "-f", "http://localhost:8080/health"]
      interval: 5s
      retries: 4
      start_period: 10s
      timeout: 2s
`
	file := mustLoad(t, yamlDoc)
	proj, err := file.Resolve(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	hc := proj.Services["api"].Healthcheck
	if hc == nil {
		t.Fatal("missing healthcheck")
	}
	if got, want := hc.Test, []string{"curl", "-f", "http://localhost:8080/health"}; !stringSliceEqual(got, want) {
		t.Fatalf("test = %v, want %v", got, want)
	}
	if hc.IntervalSec != 5 || hc.Retries != 4 || hc.StartPeriodSec != 10 || hc.TimeoutSec != 2 {
		t.Fatalf("unexpected healthcheck: %+v", hc)
	}
}

// TestResolveHealthcheck_StringForm verifies the shorthand string form
// is wrapped in `sh -c` so shell features (pipes, env expansion) work.
func TestResolveHealthcheck_StringForm(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hc2
services:
  api:
    image: ./img.qcow2
    healthcheck:
      test: "pg_isready | grep -q accepting"
`
	file := mustLoad(t, yamlDoc)
	proj, err := file.Resolve(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	hc := proj.Services["api"].Healthcheck
	if hc == nil {
		t.Fatal("missing healthcheck")
	}
	if got, want := hc.Test, []string{"sh", "-c", "pg_isready | grep -q accepting"}; !stringSliceEqual(got, want) {
		t.Fatalf("test = %v, want %v", got, want)
	}
	// Defaults apply when the compose omits the fields.
	if hc.IntervalSec != config.DefaultHealthIntervalSec {
		t.Fatalf("interval = %d, want default %d", hc.IntervalSec, config.DefaultHealthIntervalSec)
	}
	if hc.Retries != config.DefaultHealthRetries {
		t.Fatalf("retries = %d, want default %d", hc.Retries, config.DefaultHealthRetries)
	}
	if hc.TimeoutSec != config.DefaultHealthTimeoutSec {
		t.Fatalf("timeout = %d, want default %d", hc.TimeoutSec, config.DefaultHealthTimeoutSec)
	}
}

func mustLoad(t *testing.T, yamlDoc string) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return file
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseVolumeSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
	}{
		{"", defaultVolumeSizeBytes},
		{"10G", 10 * (1 << 30)},
		{"500M", 500 * (1 << 20)},
		{"1T", 1 << 40},
		{"2048K", 2048 << 10},
		{"1048576", 1 << 20},
	}
	for _, tc := range cases {
		got, err := parseVolumeSize(tc.in)
		if err != nil {
			t.Fatalf("parseVolumeSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseVolumeSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	if _, err := parseVolumeSize("bogus"); err == nil {
		t.Fatal("expected error on bogus size")
	}
	if _, err := parseVolumeSize("100"); err == nil {
		t.Fatal("expected error on size below minimum")
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
