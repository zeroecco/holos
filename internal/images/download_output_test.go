package images

import "testing"

const (
	testDownloadPath      = "/tmp/cache/debian.qcow2"
	testDownloadImagePath = "image.qcow2"
	testDownloadHash      = "abcdef0123456789extra"
	testDownloadShortHash = "abc"
	testDownloadSize      = 42 * 1024 * 1024
	testDownloadSmallSize = 1024 * 1024
)

func TestDownloadSummaryLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dest   string
		size   int64
		expect imageHash
		gotHex string
		want   string
	}{
		{
			name:   "verified sha256",
			dest:   testDownloadPath,
			size:   testDownloadSize,
			expect: imageHash{Algorithm: hashAlgorithmSHA256, Value: "expected"},
			gotHex: testDownloadHash,
			want:   "  debian.qcow2  42 MB  sha256:abcdef0123456789 (verified)\n",
		},
		{
			name:   "defaults to unverified sha256",
			dest:   testDownloadImagePath,
			size:   testDownloadSmallSize,
			gotHex: testDownloadShortHash,
			want:   "  image.qcow2  1 MB  sha256:abc (unverified)\n",
		},
		{
			name:   "uses expected algorithm",
			dest:   testDownloadImagePath,
			size:   testDownloadSmallSize,
			expect: imageHash{Algorithm: hashAlgorithmSHA512, Value: "expected"},
			gotHex: testDownloadShortHash,
			want:   "  image.qcow2  1 MB  sha512:abc (verified)\n",
		},
		{
			name:   "truncates partial megabytes",
			dest:   testDownloadImagePath,
			size:   testDownloadSize + 1024,
			gotHex: testDownloadShortHash,
			want:   "  image.qcow2  42 MB  sha256:abc (unverified)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := downloadSummaryLine(tt.dest, tt.size, tt.expect, tt.gotHex); got != tt.want {
				t.Fatalf("downloadSummaryLine = %q, want %q", got, tt.want)
			}
		})
	}
}
