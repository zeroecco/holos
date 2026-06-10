package images

import (
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

type testVerificationIdentityWant struct {
	ref    string
	path   string
	format string
}

func assertVerificationIdentity(t *testing.T, got Verification, want testVerificationIdentityWant) {
	t.Helper()

	if got.Ref != want.ref || got.Path != want.path || got.Format != want.format {
		t.Fatalf("verification identity = %+v, want %+v", got, want)
	}
}

func assertVerificationSkipped(t *testing.T, got Verification, reason string) {
	t.Helper()

	if !got.Skipped || got.Reason != reason || got.Verified {
		t.Fatalf("verification skip state = %+v, want skipped reason %q", got, reason)
	}
}

func TestVerificationHashDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hash string
		want string
	}{
		{
			name: "truncates long hash",
			hash: strings.Repeat("a", hashDisplayHexLength+1),
			want: strings.Repeat("a", hashDisplayHexLength),
		},
		{
			name: "accepts short hash",
			hash: "abc",
			want: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := Verification{Hash: tt.hash}
			if got := v.HashDisplay(); got != tt.want {
				t.Fatalf("HashDisplay = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifySkipsLocalImageWithoutChecksumMetadata(t *testing.T) {
	t.Parallel()

	got, err := Verify("local.raw", t.TempDir())
	if err != nil {
		t.Fatalf("Verify(local.raw): %v", err)
	}
	assertVerificationIdentity(t, got, testVerificationIdentityWant{
		ref:    "local.raw",
		path:   "local.raw",
		format: config.ImageFormatRaw,
	})
	assertVerificationSkipped(t, got, localImageNoChecksumReason)
}

func TestVerifySkipsRegistryImageWithoutChecksumMetadata(t *testing.T) {
	originalRegistry := Registry
	image := Image{
		Name:    "unchecked",
		Tag:     "latest",
		URL:     "https://example.com/unchecked.qcow2",
		Format:  config.ImageFormatQCOW2,
		Default: true,
	}
	Registry = append(Registry, image)
	t.Cleanup(func() { Registry = originalRegistry })

	cacheDir := t.TempDir()
	got, err := Verify("unchecked", cacheDir)
	if err != nil {
		t.Fatalf("Verify(unchecked): %v", err)
	}
	assertVerificationIdentity(t, got, testVerificationIdentityWant{
		ref:    "unchecked",
		path:   cachePath(cacheDir, &image),
		format: config.ImageFormatQCOW2,
	})
	assertVerificationSkipped(t, got, registryImageNoChecksumReason)
}
