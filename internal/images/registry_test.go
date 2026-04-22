package images

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultUser(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"alpine":          "alpine",
		"alpine:3.21":     "alpine",
		"ubuntu":          "ubuntu",
		"ubuntu:noble":    "ubuntu",
		"ubuntu:jammy":    "ubuntu",
		"debian":          "debian",
		"debian:bookworm": "debian",
		"arch":            "arch",
		"fedora":          "fedora",
		"./local.qcow2":   "", // local file → no inferred user
		"/abs/path.raw":   "",
	}
	for ref, want := range cases {
		if got := DefaultUser(ref); got != want {
			t.Errorf("DefaultUser(%q) = %q, want %q", ref, got, want)
		}
	}
}

// TestDebianUsesGenericVariant pins the Debian image URL to the
// "generic" flavour. The "nocloud" variant published alongside it
// is intentionally minimal: Debian strips out openssh-server from
// it because nocloud is meant as a base for further customisation.
// holos requires sshd in the guest for `holos exec` and ssh-based
// healthchecks, so silently regressing to nocloud would produce
// VMs where exec fails forever with "Connection reset by peer".
func TestDebianUsesGenericVariant(t *testing.T) {
	t.Parallel()

	for _, img := range Registry {
		if img.Name != "debian" {
			continue
		}
		if !strings.Contains(img.URL, "-generic-") {
			t.Errorf("debian:%s URL = %q, must use the 'generic' variant (ships openssh-server) and not 'nocloud' (does not)",
				img.Tag, img.URL)
		}
		if strings.Contains(img.URL, "-nocloud-") {
			t.Errorf("debian:%s URL = %q, the 'nocloud' variant lacks openssh-server; use 'generic' instead",
				img.Tag, img.URL)
		}
	}
}

func TestResolveKnownImages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref      string
		wantName string
		wantTag  string
	}{
		{"alpine", "alpine", "3.21"},
		{"ubuntu:noble", "ubuntu", "noble"},
		{"ubuntu:jammy", "ubuntu", "jammy"},
		{"debian", "debian", "12"},
		{"debian:bookworm", "debian", "bookworm"},
		{"arch", "arch", "latest"},
		{"fedora", "fedora", "43"},
	}

	for _, tt := range tests {
		img, err := Resolve(tt.ref)
		if err != nil {
			t.Fatalf("resolve(%q): %v", tt.ref, err)
		}
		if img == nil {
			t.Fatalf("resolve(%q): got nil, expected image", tt.ref)
		}
		if img.Name != tt.wantName {
			t.Fatalf("resolve(%q): name = %q, want %q", tt.ref, img.Name, tt.wantName)
		}
		if img.Tag != tt.wantTag {
			t.Fatalf("resolve(%q): tag = %q, want %q", tt.ref, img.Tag, tt.wantTag)
		}
		if img.URL == "" {
			t.Fatalf("resolve(%q): empty URL", tt.ref)
		}
	}
}

func TestResolveLocalPathReturnsNil(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{
		"./images/base.qcow2",
		"../output/base.qcow2",
		"/opt/images/base.qcow2",
		"myimage.qcow2",
	} {
		img, err := Resolve(ref)
		if err != nil {
			t.Fatalf("resolve(%q): unexpected error: %v", ref, err)
		}
		if img != nil {
			t.Fatalf("resolve(%q): expected nil for local path, got %+v", ref, img)
		}
	}
}

// A registry reference whose tag happens to end in a disk-image extension
// must still be routed through the registry, not treated as a local path.
// Regression guard for the earlier over-broad extension check.
func TestResolveTaggedRefWithExtensionIsNotLocal(t *testing.T) {
	t.Parallel()

	_, err := Resolve("ubuntu:noble.img")
	if err == nil {
		t.Fatal("expected unknown-image error, got nil (ref was misclassified as local path)")
	}
}

func TestResolveUnknownImage(t *testing.T) {
	t.Parallel()

	_, err := Resolve("gentoo")
	if err == nil {
		t.Fatal("expected error for unknown image")
	}
}

func TestParseRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		name string
		tag  string
	}{
		{"alpine", "alpine", ""},
		{"ubuntu:noble", "ubuntu", "noble"},
		{"debian:12", "debian", "12"},
	}

	for _, tt := range tests {
		name, tag := parseRef(tt.ref)
		if name != tt.name || tag != tt.tag {
			t.Fatalf("parseRef(%q) = (%q, %q), want (%q, %q)", tt.ref, name, tag, tt.name, tt.tag)
		}
	}
}

// TestPull_ChecksumVerification spins up a local HTTP server that returns
// a known payload. A registry-like entry with the correct hash succeeds;
// one with a wrong hash fails and leaves no partial file in the cache.
func TestPull_ChecksumVerification(t *testing.T) {
	t.Parallel()

	payload := []byte("not a real image, but deterministic bytes")
	sum := sha256.Sum256(payload)
	correctHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	cacheDir := t.TempDir()

	t.Run("correct hash succeeds", func(t *testing.T) {
		dest := filepath.Join(cacheDir, "ok.qcow2")
		if err := download(srv.URL+"/ok", dest, correctHex); err != nil {
			t.Fatalf("download with correct hash: %v", err)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("read cached: %v", err)
		}
		if string(got) != string(payload) {
			t.Fatal("cached payload does not match source")
		}
	})

	t.Run("empty hash skips verification", func(t *testing.T) {
		dest := filepath.Join(cacheDir, "skip.qcow2")
		if err := download(srv.URL+"/skip", dest, ""); err != nil {
			t.Fatalf("download without expected hash: %v", err)
		}
		if _, err := os.Stat(dest); err != nil {
			t.Fatalf("expected cached file: %v", err)
		}
	})

	t.Run("wrong hash fails and leaves no file", func(t *testing.T) {
		dest := filepath.Join(cacheDir, "bad.qcow2")
		bogus := strings.Repeat("0", 64)
		err := download(srv.URL+"/bad", dest, bogus)
		if err == nil {
			t.Fatal("expected mismatch error")
		}
		if !strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("error should mention sha256 mismatch; got %v", err)
		}
		if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
			t.Fatalf("partial file left behind after mismatch: %v", statErr)
		}
		if _, statErr := os.Stat(dest + ".part"); !os.IsNotExist(statErr) {
			t.Fatalf("temp file left behind after mismatch: %v", statErr)
		}
	})
}

// TestDownload_HeaderTimeout verifies that a server that accepts the
// TCP connection but never sends a response is aborted by the transport
// instead of hanging forever. We do this by swapping the package
// httpClient for one with aggressively short timeouts so the test
// runs in milliseconds even on slow machines. Without the fix this
// test would hang the whole go-test process.
func TestDownload_HeaderTimeout(t *testing.T) {
	blocked := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait until the test completes before returning so no
		// headers are ever sent.
		<-blocked
	}))

	// Cleanup is LIFO: unblock handlers first so the server's Close
	// (which waits for in-flight requests) does not deadlock the test.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(blocked) })

	original := httpClient
	t.Cleanup(func() { httpClient = original })
	httpClient = &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 100 * time.Millisecond,
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- download(srv.URL+"/slow", filepath.Join(t.TempDir(), "out"), "")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected header-timeout error, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("download hung past ResponseHeaderTimeout; client is missing phased timeouts")
	}
}

// TestDownload_BodyIdleTimeout proves the new watchdog catches the
// stall that happens *after* headers arrive: the server responds with
// a valid 200 and some bytes, then blocks the connection indefinitely.
// The Transport's ResponseHeaderTimeout is useless here because
// headers already landed; only the idle reader can rescue us.
func TestDownload_BodyIdleTimeout(t *testing.T) {
	unblock := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Flush headers + one byte so the Transport handshake
		// completes, then stall. This is exactly the "mirror went
		// dark mid-download" failure mode the finding described.
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("x"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-unblock
	}))

	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(unblock) })

	originalIdle := bodyIdleTimeout
	t.Cleanup(func() { bodyIdleTimeout = originalIdle })
	bodyIdleTimeout = 150 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- download(srv.URL+"/stall", filepath.Join(t.TempDir(), "out"), "")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected idle-timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "stalled") {
			t.Fatalf("error should identify the stall, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("download hung past body idle timeout; watchdog is missing")
	}
}

// TestDownload_CloseErrorVoidsCache proves that a writeback error
// surfaced at Close (ENOSPC on a full disk, a broken NFS mount, ...)
// aborts the download and removes the partial file. Without the
// check, the download would compute its sha256 over the bytes it
// managed to feed through MultiWriter, report success, and rename a
// truncated image into the cache where every future `holos up`
// reuses it forever.
func TestDownload_CloseErrorVoidsCache(t *testing.T) {
	payload := strings.Repeat("a", 1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	original := tempFileFactory
	t.Cleanup(func() { tempFileFactory = original })
	tempFileFactory = func(name string) (io.WriteCloser, error) {
		f, err := os.Create(name)
		if err != nil {
			return nil, err
		}
		return &failCloseWriter{WriteCloser: f}, nil
	}

	dest := filepath.Join(t.TempDir(), "image.qcow2")
	err := download(srv.URL, dest, "")
	if err == nil {
		t.Fatal("expected Close error to fail the download")
	}
	if !strings.Contains(err.Error(), "finalize") {
		t.Fatalf("error should mention finalize step, got: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("dest file should not exist on close failure, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(dest + ".part"); !os.IsNotExist(statErr) {
		t.Fatalf("partial file should be cleaned up, stat err=%v", statErr)
	}
}

// failCloseWriter is a WriteCloser that succeeds every Write but
// returns an error on Close. Mirrors the real "writeback surfaces at
// Close" behavior that the finding described.
type failCloseWriter struct {
	io.WriteCloser
}

func (f *failCloseWriter) Close() error {
	// Close the underlying file so the test tmpdir cleanup does not
	// race with an open handle on Windows (and stays tidy on POSIX).
	_ = f.WriteCloser.Close()
	return fmt.Errorf("simulated writeback failure")
}

func TestCacheFilename(t *testing.T) {
	t.Parallel()

	img := &Image{Name: "alpine", Tag: "3.21", URL: "https://example.com/alpine.qcow2", Format: "qcow2"}
	name := cacheFilename(img)

	if name == "" {
		t.Fatal("empty cache filename")
	}
	if len(name) < 10 {
		t.Fatalf("cache filename too short: %s", name)
	}
}
