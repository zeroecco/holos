package images

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testChecksumBaseURL    = "https://example.test"
	testChecksumFilename   = "image.qcow2"
	testChecksumImageURL   = testChecksumBaseURL + "/" + testChecksumFilename
	testChecksumSHA256Path = "/SHA256SUMS"
	testChecksumSHA512Path = "/SHA512SUMS"
	testChecksumSHA256URL  = testChecksumBaseURL + testChecksumSHA256Path
	testChecksumSHA512URL  = testChecksumBaseURL + testChecksumSHA512Path
	testChecksumUpperValue = "ABCDEF"
	testChecksumLowerValue = "abcdef"
	testChecksumActual     = "123456"
	testChecksumMismatch   = "/tmp/image.qcow2"
)

func TestExpectedHashPrefersInlineHashAndNormalizesValue(t *testing.T) {
	t.Parallel()

	img := &Image{
		URL:       testChecksumImageURL,
		SHA256:    strings.ToUpper(strings.Repeat("a", sha256HexLength)),
		SHA512:    strings.Repeat("b", sha512HexLength),
		SHA256URL: testChecksumSHA256URL,
		SHA512URL: testChecksumSHA512URL,
	}

	got, err := expectedHash(img)
	if err != nil {
		t.Fatalf("expectedHash returned error: %v", err)
	}
	if got.Algorithm != hashAlgorithmSHA256 {
		t.Fatalf("algorithm = %q, want %q", got.Algorithm, hashAlgorithmSHA256)
	}
	if want := strings.Repeat("a", sha256HexLength); got.Value != want {
		t.Fatalf("value = %q, want %q", got.Value, want)
	}
}

func TestImageChecksumAlgorithm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  Image
		want string
	}{
		{name: "sha256 inline", img: Image{SHA256: "abc"}, want: hashAlgorithmSHA256},
		{name: "sha256 url", img: Image{SHA256URL: testChecksumSHA256URL}, want: hashAlgorithmSHA256},
		{name: "sha512 inline", img: Image{SHA512: "abc"}, want: hashAlgorithmSHA512},
		{name: "sha512 url", img: Image{SHA512URL: testChecksumSHA512URL}, want: hashAlgorithmSHA512},
		{name: "sha256 wins precedence", img: Image{SHA256: "abc", SHA512: "def"}, want: hashAlgorithmSHA256},
		{name: "none", img: Image{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.img.ChecksumAlgorithm(); got != tt.want {
				t.Fatalf("ChecksumAlgorithm = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpectedHashFetchesRemoteChecksumWithAlgorithmLength(t *testing.T) {
	t.Parallel()

	sha256Value := strings.Repeat("a", sha256HexLength)
	sha512Value := strings.Repeat("b", sha512HexLength)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testChecksumSHA256Path:
			fmt.Fprintf(w, "%s  %s\n%s  wrong-length\n", sha256Value, testChecksumFilename, sha512Value)
		case testChecksumSHA512Path:
			fmt.Fprintf(w, "%s  %s\n%s  wrong-length\n", sha512Value, testChecksumFilename, sha256Value)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tests := []struct {
		name string
		img  *Image
		want imageHash
	}{
		{
			name: "sha256 url",
			img:  &Image{URL: testChecksumImageURL, SHA256URL: srv.URL + testChecksumSHA256Path},
			want: imageHash{Algorithm: hashAlgorithmSHA256, Value: sha256Value},
		},
		{
			name: "sha512 url",
			img:  &Image{URL: testChecksumImageURL, SHA512URL: srv.URL + testChecksumSHA512Path},
			want: imageHash{Algorithm: hashAlgorithmSHA512, Value: sha512Value},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expectedHash(tt.img)
			if err != nil {
				t.Fatalf("expectedHash returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expectedHash = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExpectedHashEmptyWhenNoChecksumConfigured(t *testing.T) {
	t.Parallel()

	got, err := expectedHash(&Image{URL: testChecksumImageURL})
	if err != nil {
		t.Fatalf("expectedHash returned error: %v", err)
	}
	if got != (imageHash{}) {
		t.Fatalf("expectedHash = %#v, want empty hash", got)
	}
}

func TestNewHasherSelectsSupportedAlgorithms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		algorithm string
		wantSize  int
	}{
		{name: "default", algorithm: "", wantSize: sha256.Size},
		{name: "sha256", algorithm: hashAlgorithmSHA256, wantSize: sha256.Size},
		{name: "sha512", algorithm: hashAlgorithmSHA512, wantSize: sha512.Size},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasher, err := newHasher(tt.algorithm)
			if err != nil {
				t.Fatalf("newHasher returned error: %v", err)
			}
			if got := hasher.Size(); got != tt.wantSize {
				t.Fatalf("hasher size = %d, want %d", got, tt.wantSize)
			}
		})
	}
}

func TestNewHasherRejectsUnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	if _, err := newHasher("md5"); err == nil {
		t.Fatal("newHasher accepted unsupported algorithm")
	}
}

func TestChecksumMismatchError(t *testing.T) {
	t.Parallel()

	err := checksumMismatchError(hashAlgorithmSHA256, testChecksumMismatch, testChecksumUpperValue, testChecksumActual)
	assertErrorContains(t, err,
		"sha256 mismatch for "+testChecksumMismatch,
		"expected "+testChecksumLowerValue,
		"got      "+testChecksumActual,
	)
}
