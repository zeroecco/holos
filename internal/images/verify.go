package images

import (
	"fmt"
	"os"
)

const (
	localImageNoChecksumReason    = "local image has no registry checksum metadata"
	registryImageNoChecksumReason = "registry entry has no checksum metadata"
)

// Verification describes the result of checking an image cache entry.
type Verification struct {
	Ref       string
	Path      string
	Format    string
	Algorithm string
	Hash      string
	Verified  bool
	Skipped   bool
	Reason    string
}

func (v Verification) HashDisplay() string {
	return hashDisplayPrefix(v.Hash)
}

// Verify checks an already-cached registry image (or a local path when a
// caller has no pinned hash). It never downloads missing images.
func Verify(ref string, cacheDir string) (Verification, error) {
	img, err := Resolve(ref)
	if err != nil {
		return Verification{}, err
	}
	if img == nil {
		return skippedVerification(ref, ref, inferFormat(ref), localImageNoChecksumReason), nil
	}
	expected, err := expectedHash(img)
	if err != nil {
		return Verification{}, fmt.Errorf("resolve checksum for %s: %w", ref, err)
	}
	path := cachePath(cacheDir, img)
	v := Verification{Ref: ref, Path: path, Format: img.Format, Algorithm: expected.Algorithm, Hash: expected.Value}
	if expected.Algorithm == "" {
		v.markSkipped(registryImageNoChecksumReason)
		return v, nil
	}
	if _, err := os.Stat(path); err != nil {
		return v, err
	}
	if err := verifyFile(path, expected); err != nil {
		return v, err
	}
	v.Verified = true
	return v, nil
}

func skippedVerification(ref, path, format, reason string) Verification {
	v := Verification{Ref: ref, Path: path, Format: format}
	v.markSkipped(reason)
	return v
}

func (v *Verification) markSkipped(reason string) {
	v.Skipped = true
	v.Reason = reason
}
