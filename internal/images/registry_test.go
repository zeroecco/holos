package images

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
