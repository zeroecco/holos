package images

import (
	"fmt"
	"os"
)

const (
	unknownImageErrorPrefix = "unknown image"
	imageCacheDirPerm       = os.FileMode(0o755)
)

// Resolve looks up an image reference. Accepts:
//   - "alpine"         -> default alpine image
//   - "alpine:3.21"    -> specific tag
//   - "ubuntu:noble"   -> specific tag
//   - "./path/to.qcow2" or "/abs/path" -> returned as-is (local file)
func Resolve(ref string) (*Image, error) {
	if isLocalPath(ref) {
		return nil, nil
	}

	name, tag := parseRef(ref)

	for i := range Registry {
		img := &Registry[i]
		if img.Name != name {
			continue
		}
		if (tag == "" && img.Default) || (tag != "" && img.Tag == tag) {
			return img, nil
		}
	}

	return nil, unknownImageError(name, tag)
}

func unknownImageError(name, tag string) error {
	if tag != "" {
		return fmt.Errorf("%s %q (tag %q); run 'holos images' to list available images", unknownImageErrorPrefix, name, tag)
	}
	return fmt.Errorf("%s %q; run 'holos images' to list available images", unknownImageErrorPrefix, name)
}

// Pull downloads an image to the cache directory, returning the local path.
// If already cached, re-verifies the bytes when the registry entry has
// checksum metadata before returning.
//
// When the resolved registry entry carries a non-empty SHA256, the newly
// downloaded bytes are verified against it; a mismatch deletes the partial
// file and returns an error. Cached files are trusted: the file is only
// in the cache if a prior successful pull placed it there.
func Pull(ref string, cacheDir string) (localPath string, format string, err error) {
	img, err := Resolve(ref)
	if err != nil {
		return "", "", err
	}

	if img == nil {
		return ref, inferFormat(ref), nil
	}

	if err := ensureImageCacheDir(cacheDir); err != nil {
		return "", "", fmt.Errorf("create image cache: %w", err)
	}

	cached := cachePath(cacheDir, img)

	expected, err := expectedHash(img)
	if err != nil {
		return "", "", fmt.Errorf("resolve checksum for %s: %w", ref, err)
	}

	if _, err := os.Stat(cached); err == nil {
		if cachedImageShouldBeVerified(expected) {
			if err := verifyFile(cached, expected); err != nil {
				_ = os.Remove(cached)
				fmt.Printf("cached image failed verification; re-pulling %s:%s\n", img.Name, img.Tag)
			} else {
				fmt.Printf("verified cached %s (%s:%s)\n", cached, expected.Algorithm, hashDisplayPrefix(expected.Value))
				return cached, img.Format, nil
			}
		} else {
			return cached, img.Format, nil
		}
	}

	fmt.Printf("pulling %s:%s ...\n", img.Name, img.Tag)

	if err := download(img.URL, cached, expected); err != nil {
		_ = os.Remove(cached)
		return "", "", fmt.Errorf("pull %s: %w", ref, err)
	}

	fmt.Printf("cached  %s\n", cached)
	return cached, img.Format, nil
}

func cachedImageShouldBeVerified(expected imageHash) bool {
	return expected.Algorithm != ""
}

func ensureImageCacheDir(cacheDir string) error {
	return os.MkdirAll(cacheDir, imageCacheDirPerm)
}
