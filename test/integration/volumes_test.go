//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVolumes_EndToEnd confirms that a named volume:
//   - is pre-created under state_dir/volumes/<project>/<name>.qcow2
//   - is symlinked into every instance workdir
//   - is attached to QEMU via -drive/-device with the expected serial
//   - survives `holos down` (the backing file is not removed)
func TestVolumes_EndToEnd(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("volproj", "", nil)
	img := h.fakeImage(dir, "base.qcow2")

	compose := fmt.Sprintf(`
name: volproj
volumes:
  data:
    size: 2G
services:
  db:
    image: %s
    volumes:
      - data:/var/lib/postgres
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	// Backing qcow2 lives under state_dir/volumes/<project>/<name>.qcow2.
	backing := filepath.Join(h.stateDir, "volumes", "volproj", "data.qcow2")
	if _, err := os.Stat(backing); err != nil {
		t.Fatalf("expected volume backing at %s: %v", backing, err)
	}

	// The instance workdir contains a symlink (not a real file) that
	// points at the backing qcow2. This is what lets `holos down` remove
	// the workdir without clobbering volume data.
	linkPath := filepath.Join(h.stateDir, "instances", "volproj", "db-0", "vol-data.qcow2")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected volume symlink at %s: %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink, got mode %v", linkPath, info.Mode())
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != backing {
		t.Fatalf("symlink points to %s, expected %s", target, backing)
	}

	// QEMU must be told about the volume via split drive/device args,
	// and the device must carry serial=vol-<name> so the guest gets a
	// stable /dev/disk/by-id/virtio-vol-<name> path.
	logData, _ := os.ReadFile(h.qemuLog)
	log := string(logData)
	if !strings.Contains(log, "id=vol-data,if=none") {
		t.Fatalf("expected vol-data drive in qemu args; got:\n%s", log)
	}
	if !strings.Contains(log, "virtio-blk-pci,drive=vol-data,serial=vol-data") {
		t.Fatalf("expected vol-data device with serial in qemu args; got:\n%s", log)
	}

	// Volumes persist across `down`: the backing qcow2 must still exist
	// after teardown, but the symlink (inside the instance workdir) is
	// expected to have been cleaned up along with the workdir.
	h.mustRun("down", "-f", dir+"/holos.yaml")

	if _, err := os.Stat(backing); err != nil {
		t.Fatalf("volume backing must survive down; got: %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("instance workdir symlink should be gone after down; stat returned %v", err)
	}
}

// TestVolumes_PersistAcrossRestart verifies bringing the project down and
// back up reuses the existing backing file (qemu-img create would fail if
// we weren't skipping the already-provisioned volume on the second pass).
func TestVolumes_PersistAcrossRestart(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("volpersist", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: volpersist
volumes:
  cache: {}
services:
  web:
    image: %s
    volumes:
      - cache:/var/cache
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	backing := filepath.Join(h.stateDir, "volumes", "volpersist", "cache.qcow2")

	// Simulate real data in the volume so we can prove it survives.
	if err := os.WriteFile(backing, []byte("sentinel-contents"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	h.mustRun("down", "-f", dir+"/holos.yaml")
	h.mustRun("up", "-f", dir+"/holos.yaml")

	got, err := os.ReadFile(backing)
	if err != nil {
		t.Fatalf("read backing after restart: %v", err)
	}
	if string(got) != "sentinel-contents" {
		t.Fatalf("volume data lost across down/up; got %q", string(got))
	}
}

// TestVolumes_UndeclaredReferenceFails makes sure a compose file that
// references an undeclared volume name fails validation rather than
// silently creating a bind mount against a relative path.
func TestVolumes_UndeclaredReferenceFails(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("undecl", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	// Note: `data:/mnt` with no `:./` prefix and no matching top-level
	// volume should now be an error.
	compose := fmt.Sprintf(`
name: undecl
services:
  svc:
    image: %s
    volumes:
      - data:/mnt
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	// `data:/mnt` without a top-level `data:` entry and without a `./`
	// prefix is ambiguous: it reads as a named-volume reference but
	// nothing declared one. The resolver must reject it loudly so a
	// typo (`dta` vs `data`) doesn't silently demote to a bind mount.
	_, stderr, err := h.run("validate", "-f", dir+"/holos.yaml")
	if err == nil {
		t.Fatal("expected validate to reject undeclared volume reference")
	}
	if !strings.Contains(stderr, "data") || !strings.Contains(stderr, "volume") {
		t.Fatalf("expected error to mention the volume name; got:\n%s", stderr)
	}
}

// TestVolumes_CloudInitRunCmdInjected verifies the generated seed (when
// the mock cloud-localds concatenates it) contains mkfs + fstab commands
// for every declared volume, with the stable by-id device path.
func TestVolumes_CloudInitRunCmdInjected(t *testing.T) {
	h := newHarness(t)

	dir := h.writeProject("volci", "", nil)
	img := h.fakeImage(dir, "base.qcow2")
	compose := fmt.Sprintf(`
name: volci
volumes:
  data: {}
services:
  svc:
    image: %s
    volumes:
      - data:/srv
`, img)
	if _, err := writeFile(dir, "holos.yaml", compose); err != nil {
		t.Fatal(err)
	}

	h.mustRun("up", "-f", dir+"/holos.yaml")

	seed := filepath.Join(h.stateDir, "instances", "volci", "svc-0", "seed.img")
	contents, err := os.ReadFile(seed)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	s := string(contents)

	if !strings.Contains(s, "/dev/disk/by-id/virtio-vol-data") {
		t.Fatalf("cloud-init user-data missing by-id device path:\n%s", s)
	}
	if !strings.Contains(s, "mkfs.ext4") {
		t.Fatalf("cloud-init user-data missing mkfs command:\n%s", s)
	}
	if !strings.Contains(s, "/srv") {
		t.Fatalf("cloud-init user-data missing target path:\n%s", s)
	}
}
