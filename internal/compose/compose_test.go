package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
      disk_size: 2GB
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
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("fake"), 0o600); err != nil {
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
	if got := project.Services["db"].VM.DiskSizeBytes; got != 2*(1<<30) {
		t.Fatalf("expected db disk size 2GiB, got %d", got)
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

func TestUEFIAutoEnabledWithDevices(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		uefi     bool
		devices  []ComposeDevice
		wantUEFI bool
		why      string
	}{
		{"no-devices-no-uefi", false, nil, false, "no PCI devices, no explicit flag → SeaBIOS"},
		{"explicit-uefi", true, nil, true, "operator asked for UEFI, no devices"},
		{"devices-force-uefi", false, []ComposeDevice{{PCI: "0000:01:00.0"}}, true, "PCI passthrough requires OVMF"},
		{"devices-and-explicit", true, []ComposeDevice{{PCI: "0000:01:00.0"}}, true, "both set, idempotent"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := &File{
				Name: "uefitest",
				Services: map[string]Service{
					"vm": {
						Image:   "./base.qcow2",
						VM:      VM{UEFI: c.uefi},
						Devices: c.devices,
					},
				},
			}
			project, err := file.Resolve(dir, stateDir)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := project.Services["vm"].VM.UEFI
			if got != c.wantUEFI {
				t.Errorf("%s: UEFI = %v, want %v", c.why, got, c.wantUEFI)
			}
		})
	}
}

func TestResolveRejectsInvalidCloudInitUser(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	file := &File{
		Name: "baduser",
		Services: map[string]Service{
			"vm": {
				Image: "./base.qcow2",
				CloudInit: CloudInit{
					User: "bad user",
				},
			},
		},
	}
	if _, err := file.Resolve(dir, stateDir); err == nil {
		t.Fatal("expected invalid cloud_init.user error")
	} else if !strings.Contains(err.Error(), "cloud_init.user") {
		t.Fatalf("error should name cloud_init.user, got %v", err)
	}
}

func TestResolveAcceptsComposeUserSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	file := &File{
		Name: "composeuser",
		Services: map[string]Service{
			"vm": {
				Image: "./base.qcow2",
				User:  "alpine",
			},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := project.Services["vm"].CloudInit.User; got != "alpine" {
		t.Fatalf("cloud-init user = %q, want alpine", got)
	}

	file.Services["vm"] = Service{
		Image: "./base.qcow2",
		User:  "alpine",
		CloudInit: CloudInit{
			User: "ubuntu",
		},
	}
	project, err = file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve override: %v", err)
	}
	if got := project.Services["vm"].CloudInit.User; got != "ubuntu" {
		t.Fatalf("cloud-init override user = %q, want ubuntu", got)
	}
}

func TestResolveRejectsInvalidPCIAddress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	file := &File{
		Name: "badpci",
		Services: map[string]Service{
			"vm": {
				Image:   "./base.qcow2",
				Devices: []ComposeDevice{{PCI: "01:00.8"}},
			},
		},
	}
	if _, err := file.Resolve(dir, stateDir); err == nil {
		t.Fatal("expected invalid PCI address error")
	} else if !strings.Contains(err.Error(), "pci") {
		t.Fatalf("error should name pci, got %v", err)
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

func TestDebian13AddsVGABootWorkaround(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	prewarmImageCache(t, stateDir, "debian:13")

	file := &File{
		Name: "debian13",
		Services: map[string]Service{
			"vm": {Image: "debian:13"},
		},
	}
	project, err := file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got := project.Services["vm"].VM.ExtraArgs
	if len(got) < 2 || got[0] != "-device" || got[1] != "VGA" {
		t.Fatalf("debian:13 extra args = %v, want leading -device VGA", got)
	}

	file.Services["vm"] = Service{Image: "debian:13", VM: VM{UEFI: true}}
	project, err = file.Resolve(dir, stateDir)
	if err != nil {
		t.Fatalf("resolve uefi: %v", err)
	}
	if got := project.Services["vm"].VM.ExtraArgs; len(got) != 0 {
		t.Fatalf("uefi debian:13 extra args = %v, want none", got)
	}
}

// sha256Prefix mirrors images.cacheFilename's URL-hash suffix without
// exporting it; tests only need the first 4 bytes (8 hex chars) of the
// URL's SHA-256 digest.
func sha256Prefix(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:4])
}

func prewarmImageCache(t *testing.T, stateDir, ref string) {
	t.Helper()

	img, err := images.Resolve(ref)
	if err != nil || img == nil {
		t.Fatalf("pre-warm resolve(%q): img=%v err=%v", ref, img, err)
	}
	cacheDir := filepath.Join(stateDir, "images")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(cacheDir, fmt.Sprintf("%s-%s-%s.qcow2",
		img.Name, img.Tag, sha256Prefix(img.URL)))
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTopoSortDetectsCycle(t *testing.T) {
	t.Parallel()

	file := &File{
		Name: "cycle",
		Services: map[string]Service{
			"a": {Image: "x.qcow2", DependsOn: DependsOn{"b"}},
			"b": {Image: "x.qcow2", DependsOn: DependsOn{"a"}},
		},
	}

	_, err := file.topoSort()
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestResolveAcceptsLongDependsOnSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: longdeps
services:
  web:
    image: ./base.qcow2
    depends_on:
      db:
        condition: service_healthy
        restart: true
      redis:
        condition: service_started
        required: true
  db:
    image: ./base.qcow2
  redis:
    image: ./base.qcow2
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got, want := project.ServiceOrder, []string{"db", "redis", "web"}; !stringSliceEqual(got, want) {
		t.Fatalf("service order = %v, want %v", got, want)
	}
	if got, want := project.Services["web"].DependsOn, []string{"db", "redis"}; !stringSliceEqual(got, want) {
		t.Fatalf("web depends_on = %v, want %v", got, want)
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec      string
		hostAddr  string
		host      int
		guestAddr string
		guest     int
		protocol  string
	}{
		{"8080:80", "", 8080, "", 80, "tcp"},
		{"443:443/tcp", "", 443, "", 443, "tcp"},
		{"80", "", 0, "", 80, "tcp"},
		{"127.0.0.1:8080:80", "127.0.0.1", 8080, "", 80, "tcp"},
		{"0.0.0.0:8443:443/tcp", "0.0.0.0", 8443, "", 443, "tcp"},
		{"127.0.0.1:8080:10.0.2.15:80", "127.0.0.1", 8080, "10.0.2.15", 80, "tcp"},
		{"0.0.0.0:8443:10.0.2.15:443/tcp", "0.0.0.0", 8443, "10.0.2.15", 443, "tcp"},
	}

	for _, tt := range tests {
		ports, err := parsePort(tt.spec)
		if err != nil {
			t.Fatalf("parsePort(%q): %v", tt.spec, err)
		}
		if len(ports) != 1 {
			t.Fatalf("parsePort(%q) returned %d ports, want 1", tt.spec, len(ports))
		}
		pf := ports[0]
		if pf.HostAddr != tt.hostAddr || pf.HostPort != tt.host || pf.GuestAddr != tt.guestAddr || pf.GuestPort != tt.guest || pf.Protocol != tt.protocol {
			t.Fatalf("parsePort(%q) = %+v, want host=%s:%d guest=%s:%d proto=%s",
				tt.spec, pf, tt.hostAddr, tt.host, tt.guestAddr, tt.guest, tt.protocol)
		}
	}
}

func TestParsePortRejectsInvalidAddresses(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{
		"localhost:8080:10.0.2.15:80",
		"127.0.0.1:8080:localhost:80",
		"::1:8080:10.0.2.15:80",
	} {
		if _, err := parsePort(spec); err == nil {
			t.Fatalf("parsePort(%q): expected address error", spec)
		}
	}
}

func TestParsePortRejectsIPv6WithClearError(t *testing.T) {
	t.Parallel()

	for _, spec := range []string{
		"[::1]:8080:80",
		"127.0.0.1:8080:[::1]:80",
	} {
		_, err := parsePort(spec)
		if err == nil {
			t.Fatalf("parsePort(%q): expected IPv6 error", spec)
		}
		if !strings.Contains(err.Error(), "only IPv4") {
			t.Fatalf("parsePort(%q) error = %v, want only IPv4", spec, err)
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

func TestResolveAcceptsLongPortSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: longports
services:
  web:
    image: ./base.qcow2
    ports:
      - name: web
        target: "80"
        host_ip: 127.0.0.1
        published: "8080"
        protocol: tcp
        app_protocol: http
        mode: host
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	ports := project.Services["web"].Ports
	if len(ports) != 1 {
		t.Fatalf("ports len = %d, want 1", len(ports))
	}
	got := ports[0]
	if got.Name != "web" || got.HostAddr != "127.0.0.1" || got.HostPort != 8080 || got.GuestPort != 80 || got.Protocol != "tcp" {
		t.Fatalf("resolved port = %+v, want name=web host=127.0.0.1:8080 guest=80 proto=tcp", got)
	}
}

func TestResolveAcceptsPortRangeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: portranges
services:
  web:
    image: ./base.qcow2
    ports:
      - "8080-8081:80-81"
      - target: 90
        published: "8090-8091"
        protocol: tcp
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	ports := project.Services["web"].Ports
	if len(ports) != 4 {
		t.Fatalf("ports len = %d, want 4: %+v", len(ports), ports)
	}
	for i, want := range []struct{ host, guest int }{{8080, 80}, {8081, 81}, {8090, 90}, {8091, 90}} {
		if ports[i].HostPort != want.host || ports[i].GuestPort != want.guest {
			t.Fatalf("port %d = %d:%d, want %d:%d", i, ports[i].HostPort, ports[i].GuestPort, want.host, want.guest)
		}
	}
}

func TestResolveAcceptsLabelsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: labels
services:
  map:
    image: ./base.qcow2
    labels:
      com.example.role: api
      com.example.empty: ""
  list:
    image: ./base.qcow2
    labels:
      - com.example.role=worker
      - com.example.flag
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["map"].Labels; got["com.example.role"] != "api" || got["com.example.empty"] != "" {
		t.Fatalf("map labels = %#v", got)
	}
	if got := project.Services["list"].Labels; got["com.example.role"] != "worker" || got["com.example.flag"] != "" {
		t.Fatalf("list labels = %#v", got)
	}
}

func TestResolveAcceptsLabelFileSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.env"), []byte("com.example.role=file\ncom.example.file=true\n"), 0o600); err != nil {
		t.Fatalf("write labels: %v", err)
	}
	yamlDoc := `
name: labelfile
services:
  api:
    image: ./base.qcow2
    label_file:
      - ./labels.env
    labels:
      com.example.role: inline
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	labels := project.Services["api"].Labels
	if labels["com.example.role"] != "inline" || labels["com.example.file"] != "true" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestResolveAcceptsExtraHostsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: extrahosts
services:
  map:
    image: ./base.qcow2
    extra_hosts:
      db.local: 10.0.0.10
  list:
    image: ./base.qcow2
    extra_hosts:
      - cache.local=10.0.0.11
      - api.local:10.0.0.12
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["map"].ExtraHosts["db.local"]; got != "10.0.0.10" {
		t.Fatalf("map extra host = %q, want 10.0.0.10", got)
	}
	hosts := project.Services["list"].ExtraHosts
	if hosts["cache.local"] != "10.0.0.11" || hosts["api.local"] != "10.0.0.12" {
		t.Fatalf("list extra hosts = %#v", hosts)
	}
	if hosts["map"] == "" {
		t.Fatalf("project service hosts should still be present: %#v", hosts)
	}
}

func TestResolveAcceptsHostnameSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: names
services:
  api:
    image: ./base.qcow2
    hostname: api
    domainname: example.internal
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["api"].CloudInit.Hostname; got != "api.example.internal" {
		t.Fatalf("hostname = %q, want api.example.internal", got)
	}
}

func TestResolveAcceptsScaleSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: scale
services:
  worker:
    image: ./base.qcow2
    scale: "3"
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["worker"].Replicas; got != 3 {
		t.Fatalf("replicas = %d, want 3", got)
	}
}

func TestResolveAcceptsComposeResourceSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: resources
services:
  vm:
    image: ./base.qcow2
    cpus: "2.5"
    mem_limit: 1G
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	vm := project.Services["vm"].VM
	if vm.VCPU != 3 {
		t.Fatalf("vcpu = %d, want ceil(2.5) = 3", vm.VCPU)
	}
	if vm.MemoryMB != 1024 {
		t.Fatalf("memory_mb = %d, want 1024", vm.MemoryMB)
	}
}

func TestResolveAcceptsComposeBooleanCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: booleans
services:
  vm:
    image: ./base.qcow2
    init: "true"
    privileged: "true"
    read_only: "false"
    tty: "true"
    stdin_open: "true"
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	} else if project, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	} else if got := len(project.Services["vm"].Devices); got != 0 {
		t.Fatalf("compose string devices should be compatibility metadata, got %d passthrough devices", got)
	}
}

func TestResolveAcceptsComposeLifecycleCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: lifecycle
services:
  vm:
    image: ./base.qcow2
    container_name: lifecycle-vm
    platform: linux/amd64
    pull_policy: missing
    profiles: ["debug", "local"]
    restart: unless-stopped
    stop_signal: SIGTERM
    oom_kill_disable: "true"
    pids_limit: "128"
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := len(project.Services["vm"].Devices); got != 0 {
		t.Fatalf("compose string devices should be compatibility metadata, got %d passthrough devices", got)
	}
}

func TestResolveAcceptsComposeNetworkCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: networkcompat
services:
  api:
    image: ./base.qcow2
    annotations:
      com.example.owner: platform
    dns: 1.1.1.1
    dns_search:
      - svc.local
      - example.internal
    expose:
      - "8080"
    external_links:
      - redis
    group_add:
      - dialout
      - "1000"
    links:
      - db
  worker:
    image: ./base.qcow2
    annotations:
      - com.example.role=worker
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveAcceptsComposeNetworksSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: networks
services:
  api:
    image: ./base.qcow2
    networks:
      frontend:
        aliases: [web]
        ipv4_address: 172.20.0.10
        mac_address: 02:42:ac:14:00:0a
        priority: 10.5
        gw_priority: 1.5
        backend: {}
  worker:
    image: ./base.qcow2
    networks:
      - backend
networks:
  frontend:
    driver: bridge
    driver_opts:
      com.docker.network.bridge.name: holos0
      mtu: 1500
    labels:
      com.example.tier: edge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
  backend:
    internal: true
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveAcceptsComposeConfigsAndSecretsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"base.qcow2", "app.conf", "token.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	yamlDoc := `
name: configsecrets
services:
  api:
    image: ./base.qcow2
    configs:
      - source: app_config
        target: /etc/app.conf
        uid: "1000"
        gid: "1000"
        mode: 0440
    secrets:
      - db_password
      - source: api_token
        target: /run/secrets/api_token
configs:
  app_config:
    file: ./app.conf
    labels:
      com.example.kind: config
  generated:
    content: hello
    template_driver: golang
secrets:
  db_password:
    file: ./token.txt
    labels:
      - com.example.kind=secret
    driver: example-driver
    driver_opts:
      region: test
    template_driver: golang
  api_token:
    external: true
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveAcceptsComposeBuildSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	workerDir := filepath.Join(dir, "worker")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "Dockerfile"), []byte("RUN echo api\n"), 0o600); err != nil {
		t.Fatalf("write app Dockerfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "Containerfile"), []byte("RUN echo worker\n"), 0o600); err != nil {
		t.Fatalf("write worker Dockerfile: %v", err)
	}
	yamlDoc := `
name: buildsyntax
services:
  api:
    image: ./base.qcow2
    build: ./app
  worker:
    image: ./base.qcow2
    build:
      context: ./worker
      dockerfile: Containerfile
      args:
        APP_ENV: production
      additional_contexts:
        - resources=./resources
        - alpine=docker-image://alpine:latest
      cache_from:
        - type=registry,ref=example/cache
      extra_hosts:
        - host.docker.internal=host-gateway
      isolation: default
      labels:
        com.example.role: worker
      no_cache: "true"
      pull: "true"
      provenance: mode=max
      sbom: true
      shm_size: 64M
      ssh:
        default: ~/.ssh/id_ed25519
      tags:
        - example/worker:latest
      target: prod
      ulimits:
        nofile:
          soft: 20000
          hard: 40000
  inline:
    image: ./base.qcow2
    build:
      context: ./app
      dockerfile_inline: |
        RUN echo inline
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, service := range []string{"api", "worker", "inline"} {
		if got := project.Services[service].CloudInit.RunCmd; len(got) == 0 || got[0] != "bash /var/lib/holos/build.sh" {
			t.Fatalf("%s runcmd = %v, want Dockerfile build command first", service, got)
		}
	}
	for _, wf := range project.Services["inline"].CloudInit.WriteFiles {
		if wf.Path == "/var/lib/holos/build.sh" && strings.Contains(wf.Content, "echo inline") {
			return
		}
	}
	t.Fatal("inline build script missing inline Dockerfile command")
}

func TestResolveAcceptsComposeRuntimeCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: runtimecompat
services:
  vm:
    image: ./base.qcow2
    cap_add: [NET_ADMIN]
    cap_drop:
      - MKNOD
    cgroup: private
    cgroup_parent: m-executor-abcd
    cpu_count: 2
    cpu_percent: 50
    cpu_period: "100000"
    cpu_quota: "50000"
    cpu_rt_period: 400ms
    cpu_rt_runtime: 95000
    cpu_shares: "512"
    cpuset: "0-1"
    credential_spec:
      file: my-credential-spec.json
    isolation: default
    ipc: host
    mem_reservation: 512M
    mem_swappiness: "10"
    memswap_limit: 2G
    oom_score_adj: "100"
    pid: host
    runtime: runc
    security_opt:
      - label:disable
    shm_size: 64M
    storage_opt:
      size: 1G
    sysctls:
      net.core.somaxconn: "1024"
    tmpfs:
      - /run
    ulimits:
      nofile:
        soft: 20000
        hard: 40000
      nproc: 65535
    uts: host
    userns_mode: host
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveAcceptsComposeDeploySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: deploy
services:
  worker:
    image: ./base.qcow2
    deploy:
      mode: replicated
      replicas: 3
      endpoint_mode: vip
      labels:
        com.example.role: worker
      resources:
        limits:
          cpus: "2.5"
          memory: 1G
          pids: "64"
        reservations:
          cpus: "1.0"
          memory: 512M
          generic_resources:
            - discrete_resource_spec:
                kind: FPGA
                value: "2"
          devices:
            - capabilities: [gpu]
              driver: nvidia
              count: all
              device_ids:
                - GPU-123
              options:
                - virtualization=false
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
        window: 30s
      placement:
        constraints:
          - node.labels.zone == west
      update_config:
        parallelism: 1
        delay: 10s
      rollback_config:
        parallelism: 1
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	service := project.Services["worker"]
	if service.Replicas != 3 {
		t.Fatalf("replicas = %d, want 3 from deploy.replicas", service.Replicas)
	}
	if service.VM.VCPU != 3 {
		t.Fatalf("vcpu = %d, want ceil(deploy.resources.limits.cpus)", service.VM.VCPU)
	}
	if service.VM.MemoryMB != 1024 {
		t.Fatalf("memory_mb = %d, want deploy.resources.limits.memory", service.VM.MemoryMB)
	}
}

func TestResolveAcceptsComposeMiscCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
version: "3.9"
name: misccompat
include:
  - common.yaml
services:
  vm:
    image: ./base.qcow2
    attach: "false"
    pull_policy: refresh
    pull_refresh_after: 1h30m
    blkio_config:
      weight: "300"
      weight_device:
        - path: /dev/sda
          weight: "400"
      device_read_bps:
        - path: /dev/sda
          rate: 12mb
      device_write_iops:
        - path: /dev/sda
          rate: 120
    device_cgroup_rules:
      - 'c 1:3 mr'
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
      - vendor1.com/device=gpu
      - source: /dev/fuse
        target: /dev/fuse
        permissions: rwm
    device_read_bps:
      - path: /dev/sdb
        rate: 1mb
    device_read_iops:
      - path: /dev/sdb
        rate: 100
    device_write_bps:
      - path: /dev/sdb
        rate: 2mb
    device_write_iops:
      - path: /dev/sdb
        rate: 200
    dns_opt:
      - use-vc
    develop:
      watch:
        - path: ./src
          action: sync
          include:
            - "*.go"
          target: /app
          exec:
            command: echo synced
            privileged: "true"
    extends:
      file: common.yaml
      service: base
    gpus:
      - driver: 3dfx
        count: 2
        capabilities: [gpu]
        device_ids:
          - GPU-123
        options:
          - profile=compute
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: 3
    mac_address: 02:42:ac:11:00:02
    post_start:
      - command: echo started
        user: root
        privileged: "true"
        environment:
          HOOK: post
    pre_stop:
      - command: echo stopping
    provider:
      type: awesomecloud
      options:
        size: small
        regions:
          - us-west
          - us-east
    use_api_socket: true
    volumes_from:
      - db:ro
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestResolveAcceptsComposeModelsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: models
services:
  app:
    image: ./base.qcow2
    models:
      llm:
        endpoint_var: MODEL_URL
        model_var: MODEL_NAME
  worker:
    image: ./base.qcow2
    models:
      - embeddings
models:
  llm:
    name: logical-llm
    model: ai/example
    context_size: 4096
    runtime_flags:
      - --threads=4
  embeddings:
    model: ai/embed
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestLoadAcceptsComposeExtensionFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
x-project: common metadata
name: extensions
services:
  vm:
    x-service:
      team: platform
    image: ./base.qcow2
    build:
      context: .
      dockerfile_inline: |
        RUN echo extension
      x-bake:
        platforms:
          - linux/amd64
    deploy:
      x-swarm-note: ignored
      resources:
        limits:
          cpus: "1.5"
networks:
  default:
    x-network: ignored
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["vm"].VM.VCPU; got != 2 {
		t.Fatalf("vcpu = %d, want deploy cpu fallback after stripping extensions", got)
	}
}

func TestLoadAcceptsComposeResetAndOverrideTags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: tags
services:
  vm:
    image: ./base.qcow2
    ports: !override
      - "8080:80"
    environment: !reset null
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := len(project.Services["vm"].Ports); got != 1 {
		t.Fatalf("ports len = %d, want 1", got)
	}
}

func TestLoadAcceptsComposeAnchorsAndAliases(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: anchors
x-common-env: &common-env
  APP_ENV: test
services:
  vm:
    image: ./base.qcow2
    environment: *common-env
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, wf := range project.Services["vm"].CloudInit.WriteFiles {
		if wf.Path == "/etc/environment" && strings.Contains(wf.Content, `APP_ENV="test"`) {
			return
		}
	}
	t.Fatal("environment from alias was not rendered")
}

func TestResolveAcceptsComposeIncludeAndModeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "labels.env"), []byte("com.example.mode=host\n"), 0o600); err != nil {
		t.Fatalf("write labels: %v", err)
	}
	yamlDoc := `
name: includecompat
include:
  - path:
      - ../commons/compose.yaml
      - ./override.yaml
    project_directory: ..
    env_file:
      - ../commons/.env
  - ./simple.yaml
services:
  vm:
    image: ./base.qcow2
    label_file:
      - ./labels.env
    network_mode: host
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
}

func TestLoadMergesExistingComposeIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	included := `
services:
  db:
    image: ./base.qcow2
volumes:
  data:
    size: 3G
`
	if err := os.WriteFile(filepath.Join(dir, "common.yaml"), []byte(included), 0o600); err != nil {
		t.Fatalf("write include: %v", err)
	}
	main := `
name: includemerge
include:
  - ./common.yaml
services:
  api:
    image: ./base.qcow2
    depends_on: [db]
    volumes:
      - data:/data
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(main), 0o600); err != nil {
		t.Fatalf("write main: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, ok := project.Services["db"]; !ok {
		t.Fatalf("included service db missing: %#v", project.Services)
	}
	if got := project.Volumes["data"].SizeBytes; got != 3*(1<<30) {
		t.Fatalf("included volume size = %d, want 3G", got)
	}
}

func TestLoadIncludeProjectDirectoryResolvesServicePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commonDir := filepath.Join(dir, "common")
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatalf("mkdir common: %v", err)
	}
	imagePath := filepath.Join(commonDir, "base.qcow2")
	if err := os.WriteFile(imagePath, []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	included := `
services:
  db:
    image: ./base.qcow2
`
	if err := os.WriteFile(filepath.Join(commonDir, "compose.yaml"), []byte(included), 0o600); err != nil {
		t.Fatalf("write include: %v", err)
	}
	main := `
name: includeprojectdir
include:
  - path: compose.yaml
    project_directory: ./common
services:
  api:
    image: ./common/base.qcow2
    depends_on: [db]
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(main), 0o600); err != nil {
		t.Fatalf("write main: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Services["db"].Image; got != imagePath {
		t.Fatalf("included service image = %q, want %q", got, imagePath)
	}
}

func TestResolveAcceptsComposeVolumeMetadataSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: volumemeta
services:
  db:
    image: ./base.qcow2
    volumes:
      - data:/var/lib/data
volumes:
  data:
    name: external-data
    driver: local
    driver_opts:
      type: none
    external:
      name: shared-data
    labels:
      com.example.role: data
    size: 20G
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := project.Volumes["data"].SizeBytes; got != 20*(1<<30) {
		t.Fatalf("volume size = %d, want 20G", got)
	}
}

func TestResolveAcceptsComposeServiceLongVolumeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	bindDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(bindDir, 0o755); err != nil {
		t.Fatalf("mkdir bind: %v", err)
	}
	yamlDoc := `
name: longvolumes
services:
  db:
    image: ./base.qcow2
    volumes:
      - type: volume
        source: data
        target: /var/lib/data
        read_only: true
        volume:
          nocopy: true
      - type: bind
        source: ./data
        target: /mnt/data
        bind:
          create_host_path: true
volumes:
  data:
    size: 5G
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	mounts := project.Services["db"].Mounts
	if len(mounts) != 2 {
		t.Fatalf("mounts len = %d, want 2: %#v", len(mounts), mounts)
	}
	if got := mounts[0]; got.Kind != config.MountKindVolume || got.VolumeName != "data" || got.Target != "/var/lib/data" || !got.ReadOnly {
		t.Fatalf("volume mount = %+v", got)
	}
	if got := mounts[1]; got.Kind != config.MountKindBind || got.Source != bindDir || got.Target != "/mnt/data" || got.ReadOnly {
		t.Fatalf("bind mount = %+v", got)
	}
}

func TestResolveAcceptsEnvironmentSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: env
services:
  map:
    image: ./base.qcow2
    environment:
      RACK_ENV: development
      SHOW: "true"
      USER_INPUT:
  list:
    image: ./base.qcow2
    environment:
      - RACK_ENV=production
      - SHOW=false
      - USER_INPUT
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertEnvFile := func(service string, want []string) {
		t.Helper()
		for _, wf := range project.Services[service].CloudInit.WriteFiles {
			if wf.Path == "/etc/environment" {
				for _, line := range want {
					if !strings.Contains(wf.Content, line+"\n") {
						t.Fatalf("%s /etc/environment missing %q:\n%s", service, line, wf.Content)
					}
				}
				return
			}
		}
		t.Fatalf("%s missing /etc/environment write file", service)
	}
	assertEnvFile("map", []string{`RACK_ENV="development"`, `SHOW="true"`})
	assertEnvFile("list", []string{`RACK_ENV="production"`, `SHOW="false"`})
}

func TestResolveAcceptsEnvFileSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "base.env"), []byte("RACK_ENV=from-file\nSHOW=false\n"), 0o600); err != nil {
		t.Fatalf("write base env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override.env"), []byte("SHOW=true\nEXTRA=1\n"), 0o600); err != nil {
		t.Fatalf("write override env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw.env"), []byte("RAW=$NOT_EXPANDED\n"), 0o600); err != nil {
		t.Fatalf("write raw env: %v", err)
	}
	yamlDoc := `
name: envfile
services:
  api:
    image: ./base.qcow2
    env_file:
      - ./base.env
      - path: ./override.env
        required: true
      - path: ./missing.env
        required: "false"
      - path: ./raw.env
        format: raw
    environment:
      RACK_ENV: inline
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for _, wf := range project.Services["api"].CloudInit.WriteFiles {
		if wf.Path != "/etc/environment" {
			continue
		}
		for _, line := range []string{`RACK_ENV="inline"`, `SHOW="true"`, `EXTRA="1"`, `RAW="$NOT_EXPANDED"`} {
			if !strings.Contains(wf.Content, line+"\n") {
				t.Fatalf("/etc/environment missing %q:\n%s", line, wf.Content)
			}
		}
		return
	}
	t.Fatal("missing /etc/environment write file")
}

func TestResolveRejectsUnsupportedEnvFileFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vars.env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	yamlDoc := `
name: badenvformat
services:
  api:
    image: ./base.qcow2
    env_file:
      - path: ./vars.env
        format: shell
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err == nil {
		t.Fatal("expected unsupported format error")
	} else if !strings.Contains(err.Error(), `env_file format "shell" is unsupported`) {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveAcceptsCommandAndEntrypointSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("not really qcow2"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	yamlDoc := `
name: commands
services:
  string:
    image: ./base.qcow2
    command: echo hello
    entrypoint: /bin/sh -c
    working_dir: /srv/app
  list:
    image: ./base.qcow2
    command: ["echo", "hello world"]
    entrypoint: ["/usr/bin/env"]
  hooks:
    image: ./base.qcow2
    post_start:
      - command: echo started
        working_dir: /srv/app
        environment:
          HOOK: post
      - command: ["touch", "/tmp/ready"]
    pre_stop:
      - command: echo stopping
`
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got, want := project.Services["string"].CloudInit.RunCmd, []string{"cd /srv/app && /bin/sh -c echo hello"}; !stringSliceEqual(got, want) {
		t.Fatalf("string runcmd = %v, want %v", got, want)
	}
	if got, want := project.Services["list"].CloudInit.RunCmd, []string{"/usr/bin/env echo 'hello world'"}; !stringSliceEqual(got, want) {
		t.Fatalf("list runcmd = %v, want %v", got, want)
	}
	if got, want := project.Services["hooks"].CloudInit.RunCmd, []string{
		"HOOK=post cd /srv/app && echo started",
		"touch /tmp/ready",
		"echo stopping",
	}; !stringSliceEqual(got, want) {
		t.Fatalf("hooks runcmd = %v, want %v", got, want)
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

// TestParseVolume_RejectsUnknownMode pins the allow-list contract on
// the third ":mode" field. Before this change anything that wasn't
// exactly "ro" silently parsed as read-write, so a typo like
// `:readonly` or docker-compose's `:rw,Z` delivered a writable mount
// without any signal to the operator. The fix is to fail loudly for
// both bind mounts and named volumes; the test exercises both paths
// because the code branches on the declared map before validation.
func TestParseVolume_RejectsUnknownMode(t *testing.T) {
	t.Parallel()

	declared := map[string]Volume{"data": {Size: "1G"}}

	cases := []struct {
		name string
		spec string
		decl map[string]Volume
	}{
		{"bind readonly-typo", "./data:/var/lib/db:readonly", nil},
		{"bind r0-typo", "./data:/var/lib/db:r0", nil},
		{"bind docker-style-z", "./data:/var/lib/db:Z", nil},
		{"named readonly-typo", "data:/var/lib/db:readonly", declared},
		{"named empty-mode", "data:/var/lib/db:", declared},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseVolume(tc.spec, t.TempDir(), tc.decl)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.spec)
			}
			if !strings.Contains(err.Error(), "unknown mode") {
				t.Fatalf("error should call out unknown mode, got: %v", err)
			}
		})
	}
}

// TestParseVolume_AcceptsExplicitRW covers the symmetric side: `:rw`
// is equivalent to no mode suffix and must not be rejected by the
// new allow-list. Users migrating from docker-compose files that
// spell the mode out shouldn't need to strip it.
func TestParseVolume_AcceptsExplicitRW(t *testing.T) {
	t.Parallel()

	mount, err := parseVolume("./data:/var/lib/db:rw", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.ReadOnly {
		t.Fatalf("`:rw` must parse as writable, got ReadOnly=true")
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
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "img.qcow2"), []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	proj, err := file.Resolve(baseDir, t.TempDir())
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
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "img.qcow2"), []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	proj, err := file.Resolve(baseDir, t.TempDir())
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

func TestResolveHealthcheckDisabledForms(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"disable true": `
name: hcoff
services:
  api:
    image: ./img.qcow2
    healthcheck:
      disable: true
`,
		"none test": `
name: hcoff
services:
  api:
    image: ./img.qcow2
    healthcheck:
      test: ["NONE"]
`,
	}
	for name, yamlDoc := range cases {
		t.Run(name, func(t *testing.T) {
			file := mustLoad(t, yamlDoc)
			baseDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(baseDir, "img.qcow2"), []byte("fake"), 0o600); err != nil {
				t.Fatal(err)
			}
			proj, err := file.Resolve(baseDir, t.TempDir())
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if hc := proj.Services["api"].Healthcheck; hc != nil {
				t.Fatalf("healthcheck = %+v, want nil", hc)
			}
		})
	}
}

func TestResolveAcceptsHealthcheckStartInterval(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hcinterval
services:
  api:
    image: ./img.qcow2
    healthcheck:
      test: ["CMD", "true"]
      interval: 2s
      start_interval: 1s
`
	file := mustLoad(t, yamlDoc)
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "img.qcow2"), []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Resolve(baseDir, t.TempDir()); err != nil {
		t.Fatalf("resolve: %v", err)
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
		{"2GB", 2 * (1 << 30)},
		{"500M", 500 * (1 << 20)},
		{"512MB", 512 * (1 << 20)},
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
			"a": {Image: "x.qcow2", DependsOn: DependsOn{"nonexistent"}},
		},
	}
	if err := file.validate(); err == nil {
		t.Fatal("expected validation error for missing dependency")
	}
}

// TestResolveValidatesManifest pins the contract that compose
// resolution runs every resolved service through Manifest.Validate
// before returning. Without this, holos validate would happily accept
// memory_mb: -1 (later panicking deep in the runtime) and out-of-range
// host ports (later silently misconfiguring qemu user-net).
func TestResolveValidatesManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	imagePath := filepath.Join(dir, "base.qcow2")
	if err := os.WriteFile(imagePath, []byte("fake"), 0o600); err != nil {
		t.Fatalf("seed image: %v", err)
	}

	cases := map[string]string{
		"negative memory": `
name: bad
services:
  vm:
    image: ./base.qcow2
    vm:
      memory_mb: -1
`,
		"tiny disk": `
name: bad
services:
  vm:
    image: ./base.qcow2
    vm:
      disk_size: 100
`,
		"host port out of range": `
name: bad
services:
  vm:
    image: ./base.qcow2
    ports:
      - "99999:80"
`,
		"negative replicas": `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: -1
`,
		"replicas above cap": `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 100000
`,
		"project replicas exceed subnet": `
name: bad
services:
  a:
    image: ./base.qcow2
    replicas: 200
  b:
    image: ./base.qcow2
    replicas: 100
`,
		"static host port overflows across replicas": `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 2
    ports:
      - "65535:80"
`,
		// 8080:80 and 8081:81 look disjoint on paper, but the
		// runtime shifts both by the replica index, so replica 1
		// tries to bind 8081 for *both* mappings. Pre-fix this
		// slipped through validation and blew up mid-`holos up`
		// with an opaque bind error.
		"static host ports collide after replica offset": `
name: bad
services:
  vm:
    image: ./base.qcow2
    replicas: 2
    ports:
      - "8080:80"
      - "8081:81"
`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			file, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if _, err := file.Resolve(dir, dir); err == nil {
				t.Fatalf("expected resolve error for %q, got nil", name)
			}
		})
	}
}

// TestHealthcheckRejectsUnknownFields pins that typos inside the
// healthcheck block surface as an error rather than being silently
// dropped. The outer Load() uses KnownFields(true), but the custom
// Healthcheck.UnmarshalYAML has to re-enforce it because
// yaml.Node.Decode has no strict-fields toggle.
func TestHealthcheckRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	yamlDoc := `
name: hctypo
services:
  api:
    image: ./img.qcow2
    healthcheck:
      test: ["true"]
      retriez: 3
`
	dir := t.TempDir()
	path := filepath.Join(dir, "holos.yaml")
	if err := os.WriteFile(path, []byte(yamlDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected Load to reject typo'd healthcheck field")
	} else if !strings.Contains(err.Error(), "retriez") {
		t.Fatalf("error should name the offending field, got: %v", err)
	}
}

// TestResolveRejectsMissingLocalImage pins the contract that a
// compose file pointing at a local qcow2/raw that is not on disk is
// rejected at resolution time, which is what `holos validate` runs.
// Without this the failure surfaces much later inside qemu-img in
// `holos up`, and users reasonably assume `validate` caught anything
// it would.
func TestResolveRejectsMissingLocalImage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")
	body := `
name: missing
services:
  vm:
    image: ./missing.qcow2
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	file, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := file.Resolve(dir, dir); err == nil {
		t.Fatal("expected missing-image error, got nil")
	} else if !strings.Contains(err.Error(), "missing.qcow2") {
		t.Fatalf("error should name the missing file, got %v", err)
	}
}

// TestLoadRejectsUnknownFields ensures the strict YAML decoder catches
// typos that previously slipped through silently. Each case is the
// minimum YAML needed to elicit the misspelled key, asserting against
// the Go field that should have caught it.
func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"top level typo": `
name: typo
servicez:
  vm:
    image: ./base.qcow2
`,
		"service-level typo": `
name: typo
services:
  vm:
    image: ./base.qcow2
    portz:
      - "8080:80"
`,
		"nested vm typo": `
name: typo
services:
  vm:
    image: ./base.qcow2
    vm:
      memry_mb: 512
`,
	}

	dir := t.TempDir()
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, err := Load(path); err == nil {
				t.Fatalf("expected unknown-field error for %q, got nil", name)
			}
		})
	}
}
