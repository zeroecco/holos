package images

import (
	"strings"
	"testing"
)

const (
	testRegistryKnownImageRef = "alpine"
	testRegistryLocalImageRef = "./local.qcow2"
	testRegistryMissingRef    = "missing:image"
)

func TestDefaultUser(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  string
		want string
	}{
		{name: "alpine default", ref: "alpine", want: "alpine"},
		{name: "alpine tagged", ref: "alpine:3.21", want: "alpine"},
		{name: "ubuntu default", ref: "ubuntu", want: "ubuntu"},
		{name: "ubuntu noble", ref: "ubuntu:noble", want: "ubuntu"},
		{name: "ubuntu jammy", ref: "ubuntu:jammy", want: "ubuntu"},
		{name: "ubuntu resolute", ref: "ubuntu:resolute", want: "ubuntu"},
		{name: "ubuntu numeric", ref: "ubuntu:26", want: "ubuntu"},
		{name: "debian default", ref: "debian", want: "debian"},
		{name: "debian bookworm", ref: "debian:bookworm", want: "debian"},
		{name: "debian trixie", ref: "debian:trixie", want: "debian"},
		{name: "arch", ref: "arch", want: "arch"},
		{name: "fedora", ref: "fedora", want: "fedora"},
		{name: "almalinux", ref: "almalinux", want: "almalinux"},
		{name: "rocky", ref: "rocky", want: "rocky"},
		{name: "centos stream default", ref: "centos-stream", want: "cloud-user"},
		{name: "centos stream 10", ref: "centos-stream:10", want: "cloud-user"},
		{name: "relative local image", ref: "./local.qcow2"},
		{name: "absolute local image", ref: "/abs/path.raw"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := DefaultUser(tc.ref); got != tc.want {
				t.Errorf("DefaultUser(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestResolveMetadataImage(t *testing.T) {
	t.Parallel()

	img, ok := resolveMetadataImage(testRegistryKnownImageRef)
	if !ok {
		t.Fatalf("resolveMetadataImage(%s) ok = false, want true", testRegistryKnownImageRef)
	}
	if img.Name != testRegistryKnownImageRef {
		t.Fatalf("resolveMetadataImage(%s) name = %q, want %s", testRegistryKnownImageRef, img.Name, testRegistryKnownImageRef)
	}

	img, ok = resolveMetadataImage(testRegistryLocalImageRef)
	if ok || img != nil {
		t.Fatalf("resolveMetadataImage(%s) = (%+v, %v), want nil,false", testRegistryLocalImageRef, img, ok)
	}

	img, ok = resolveMetadataImage(testRegistryMissingRef)
	if ok || img != nil {
		t.Fatalf("resolveMetadataImage(%s) = (%+v, %v), want nil,false", testRegistryMissingRef, img, ok)
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
		assertDebianURLContains(t, img, "-generic-", "must use the 'generic' variant (ships openssh-server) and not 'nocloud' (does not)")
		assertDebianURLOmits(t, img, "-nocloud-", "the 'nocloud' variant lacks openssh-server; use 'generic' instead")
	}
}

func assertDebianURLContains(t *testing.T, img Image, want, reason string) {
	t.Helper()

	if !strings.Contains(img.URL, want) {
		t.Errorf("debian:%s URL = %q, %s", img.Tag, img.URL, reason)
	}
}

func assertDebianURLOmits(t *testing.T, img Image, forbidden, reason string) {
	t.Helper()

	if strings.Contains(img.URL, forbidden) {
		t.Errorf("debian:%s URL = %q, %s", img.Tag, img.URL, reason)
	}
}

func TestRequiresVGA(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "debian 13", ref: "debian:13", want: true},
		{name: "debian trixie", ref: "debian:trixie", want: true},
		{name: "debian default", ref: "debian"},
		{name: "debian bookworm", ref: "debian:bookworm"},
		{name: "rocky default", ref: "rocky", want: true},
		{name: "rocky 10", ref: "rocky:10", want: true},
		{name: "rocky 9", ref: "rocky:9"},
		{name: "ubuntu noble", ref: "ubuntu:noble"},
		{name: "local image", ref: "./local.qcow2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := RequiresVGA(tc.ref); got != tc.want {
				t.Errorf("RequiresVGA(%q) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

func TestMinMemoryMB(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  string
		want int
	}{
		{name: "centos stream default", ref: "centos-stream", want: 1024},
		{name: "centos stream 10", ref: "centos-stream:10", want: 1024},
		{name: "ubuntu noble", ref: "ubuntu:noble"},
		{name: "local image", ref: "./local.qcow2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := MinMemoryMB(tc.ref); got != tc.want {
				t.Errorf("MinMemoryMB(%q) = %d, want %d", tc.ref, got, tc.want)
			}
		})
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
		{"ubuntu:26", "ubuntu", "26"},
		{"ubuntu:26.04", "ubuntu", "26.04"},
		{"ubuntu:jammy", "ubuntu", "jammy"},
		{"debian", "debian", "12"},
		{"debian:bookworm", "debian", "bookworm"},
		{"debian:13", "debian", "13"},
		{"arch", "arch", "latest"},
		{"fedora", "fedora", "44"},
		{"fedora:43", "fedora", "43"},
		{"almalinux", "almalinux", "10"},
		{"almalinux:9", "almalinux", "9"},
		{"rocky", "rocky", "10"},
		{"rocky:9", "rocky", "9"},
		{"centos-stream", "centos-stream", "10"},
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
	assertErrorContains(t, err, unknownImageErrorPrefix)
}

func TestCachedImageShouldBeVerified(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected imageHash
		want     bool
	}{
		{name: "checksum algorithm present", expected: imageHash{Algorithm: hashAlgorithmSHA256}, want: true},
		{name: "checksum value alone is not enough", expected: imageHash{Value: "abc"}},
		{name: "no checksum metadata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cachedImageShouldBeVerified(tt.expected); got != tt.want {
				t.Fatalf("cachedImageShouldBeVerified(%+v) = %v, want %v", tt.expected, got, tt.want)
			}
		})
	}
}

func TestResolveUnknownImage(t *testing.T) {
	t.Parallel()

	_, err := Resolve("gentoo")
	assertErrorContains(t, err, unknownImageErrorPrefix)
}

func TestUnknownImageError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  string
		tag  string
		want string
	}{
		{
			name: "name only",
			img:  "gentoo",
			want: "unknown image \"gentoo\"; run 'holos images' to list available images",
		},
		{
			name: "tagged",
			img:  "ubuntu",
			tag:  "missing",
			want: "unknown image \"ubuntu\" (tag \"missing\"); run 'holos images' to list available images",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := unknownImageError(tt.img, tt.tag)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("unknownImageError = %v, want %q", err, tt.want)
			}
		})
	}
}
