package images

import (
	"testing"
)

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
