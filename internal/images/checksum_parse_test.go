package images

import (
	"strings"
	"testing"
)

func testChecksum(fill string) string {
	return strings.Repeat(fill, sha256HexLength)
}

func TestFindChecksumPrefersMatchingFilename(t *testing.T) {
	t.Parallel()

	other := testChecksum("a")
	want := testChecksum("b")
	data := other + "  other.qcow2\n" + want + " *target.qcow2\n"
	if got := findChecksum(data, "target.qcow2", sha256HexLength); got != want {
		t.Fatalf("findChecksum = %q, want %q", got, want)
	}
}

func TestFindChecksumRequiresExactFilenameMatch(t *testing.T) {
	t.Parallel()

	partial := testChecksum("a")
	want := testChecksum("b")
	data := partial + "  target.qcow2.old\n" + want + "  target.qcow2\n"
	if got := findChecksum(data, "target.qcow2", sha256HexLength); got != want {
		t.Fatalf("findChecksum = %q, want %q", got, want)
	}
}

func TestFindChecksumAcceptsSingleHashManifest(t *testing.T) {
	t.Parallel()

	want := testChecksum("A")
	if got := findChecksum("SHA256 ("+want+")\n", "image.qcow2", sha256HexLength); got != strings.ToLower(want) {
		t.Fatalf("findChecksum = %q, want %q", got, strings.ToLower(want))
	}
}

func TestFindChecksumAcceptsRepeatedHashManifest(t *testing.T) {
	t.Parallel()

	want := testChecksum("a")
	data := want + "  image.qcow2\n" + want + "  image.qcow2.gz\n"
	if got := findChecksum(data, "missing.qcow2", sha256HexLength); got != want {
		t.Fatalf("findChecksum repeated hash manifest = %q, want %q", got, want)
	}
}

func TestFindChecksumRejectsAmbiguousManifestWithoutFilename(t *testing.T) {
	t.Parallel()

	data := testChecksum("a") + "\n" + testChecksum("b") + "\n"
	if got := findChecksum(data, "missing.qcow2", sha256HexLength); got != "" {
		t.Fatalf("findChecksum ambiguous manifest = %q, want empty", got)
	}
}

func TestFindChecksumIgnoresInvalidHashFields(t *testing.T) {
	t.Parallel()

	data := strings.Repeat("z", sha256HexLength) + "  image.qcow2\n" +
		testChecksum("a")[:sha256HexLength-1] + "  other.qcow2\n"
	if got := findChecksum(data, "image.qcow2", sha256HexLength); got != "" {
		t.Fatalf("findChecksum invalid hash fields = %q, want empty", got)
	}
}

func TestFindChecksumNormalizesCommonFilenameTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "gnu binary marker", data: testChecksum("a") + " *image.qcow2", want: testChecksum("a")},
		{name: "bsd filename parens", data: "SHA256 (image.qcow2) = " + testChecksum("b"), want: testChecksum("b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := findChecksum(tt.data, "image.qcow2", sha256HexLength); got != tt.want {
				t.Fatalf("findChecksum = %q, want %q", got, tt.want)
			}
		})
	}
}
