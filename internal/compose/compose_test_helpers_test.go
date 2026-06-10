package compose

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testComposeAppName      = "testapp"
	testComposeDBService    = "db"
	testComposeAPIService   = "api"
	testComposeWebService   = "web"
	testComposeWebHostPort  = 8080
	testComposeWebGuestPort = 80
	testComposeWebMount     = "/srv/www"
	testComposeWebReplicas  = 2
	testComposeDBDiskBytes  = 2 * (1 << 30)
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
    mac_address: 02:42:ac:11:00:02
    depends_on:
      - api
    ports:
      - "8080:80"
    volumes:
      - ./www:/srv/www:ro
`

type testImageResolver struct {
	users       map[string]string
	osFamilies  map[string]string
	minMemory   map[string]int
	requiresVGA map[string]bool
}

func (r testImageResolver) Pull(ref string, cacheDir string) (string, string, error) {
	if strings.HasPrefix(ref, ".") || filepath.IsAbs(ref) {
		return ref, config.ImageFormatQCOW2, nil
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	name := strings.NewReplacer(":", "-", "/", "-").Replace(ref) + ".qcow2"
	path := filepath.Join(cacheDir, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return "", "", err
	}
	return path, config.ImageFormatQCOW2, nil
}

func (r testImageResolver) OSFamily(ref string) string {
	return r.osFamilies[ref]
}

func (r testImageResolver) MinMemoryMB(ref string) int {
	return r.minMemory[ref]
}

func (r testImageResolver) DefaultUser(ref string) string {
	return r.users[ref]
}

func (r testImageResolver) RequiresVGA(ref string) bool {
	return r.requiresVGA[ref]
}

var composeTestImages = testImageResolver{
	users: map[string]string{
		"alpine":          "alpine",
		"debian:13":       "debian",
		"debian:bookworm": "debian",
		"fedora":          "fedora",
		"rocky:10":        "rocky",
	},
	osFamilies: map[string]string{
		"alpine":          config.ImageOSOpenRC,
		"debian:13":       config.ImageOSSystemd,
		"debian:bookworm": config.ImageOSSystemd,
		"fedora":          config.ImageOSSystemd,
		"rocky:10":        config.ImageOSSystemd,
		"centos-stream":   config.ImageOSSystemd,
	},
	minMemory: map[string]int{
		"centos-stream": 1024,
	},
	requiresVGA: map[string]bool{
		"debian:13": true,
		"rocky:10":  true,
	},
}

func writeTestFile(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func writeTestImage(t *testing.T, dir string) string {
	t.Helper()

	return writeTestFile(t, dir, "base.qcow2", "not really qcow2")
}

func testStateDir(root string) string {
	return filepath.Join(root, "state")
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertWriteFileContains(t *testing.T, service, path string, writeFiles []config.WriteFile, wants ...string) {
	t.Helper()

	for _, wf := range writeFiles {
		if wf.Path != path {
			continue
		}
		for _, want := range wants {
			if !strings.Contains(wf.Content, want) {
				t.Fatalf("%s %s missing %q:\n%s", service, path, want, wf.Content)
			}
		}
		return
	}
	t.Fatalf("%s missing %s write file", service, path)
}

func loadTestCompose(t *testing.T, dir, yamlDoc string) *File {
	t.Helper()

	path := writeTestFile(t, dir, "holos.yaml", yamlDoc)
	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return file
}

func resolveTestCompose(t *testing.T, dir, yamlDoc string) *Project {
	t.Helper()

	file := loadTestCompose(t, dir, yamlDoc)
	project, err := file.Resolve(dir, dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return project
}

func stringSliceEqual(a, b []string) bool {
	return slices.Equal(a, b)
}

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !stringSliceEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertIntSliceEqual(t *testing.T, name string, got, want []int) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertStringSliceFirst(t *testing.T, name string, got []string, want string) {
	t.Helper()

	if len(got) == 0 {
		t.Fatalf("%s = %v, want first entry %q", name, got, want)
	}
	if got[0] != want {
		t.Fatalf("%s first = %q, want %q: %v", name, got[0], want, got)
	}
}

func assertStringSlicePrefix(t *testing.T, name string, got, wantPrefix []string) {
	t.Helper()

	if len(got) < len(wantPrefix) {
		t.Fatalf("%s = %v, want prefix %v", name, got, wantPrefix)
	}
	for i, want := range wantPrefix {
		if got[i] != want {
			t.Fatalf("%s[%d] = %q, want %q; full value %v", name, i, got[i], want, got)
		}
	}
}

type testPortForwardWant struct {
	hostPort  int
	guestPort int
	protocol  string
}

func assertPortForward(t *testing.T, name string, got config.PortForward, want testPortForwardWant) {
	t.Helper()

	if got.HostPort != want.hostPort || got.GuestPort != want.guestPort || got.Protocol != want.protocol {
		t.Fatalf("%s port = %+v, want %+v", name, got, want)
	}
}

func assertPortForwards(t *testing.T, name string, got, want []config.PortForward) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %+v, want %+v", name, got, want)
	}
}
