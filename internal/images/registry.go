package images

import (
	"fmt"
	"os"
)

const (
	unknownImageErrorPrefix = "unknown image"
	imageCacheDirPerm       = os.FileMode(0o700)
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
// Ubuntu release selections are saved after successful verification so a
// later pull reuses the same build even if upstream publishes a newer one.
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

	entry := img
	newSelection := false
	if img.UbuntuRelease != "" {
		img, newSelection, err = ubuntuImageForPull(img, cacheDir)
		if err != nil {
			return "", "", fmt.Errorf("resolve image for %s: %w", ref, err)
		}
	}

	cached, format, err := pullResolvedImage(ref, img, cacheDir)
	if err != nil || !newSelection {
		return cached, format, err
	}
	selected, err := commitUbuntuSelection(cacheDir, entry, img)
	if err != nil {
		return "", "", err
	}
	if selected.URL != img.URL || selected.SHA256 != img.SHA256 {
		// A concurrent pull won the selection. Verify its bytes before returning
		// that build, retaining our download for any existing overlays.
		return pullResolvedImage(ref, selected, cacheDir)
	}
	return cached, format, nil
}

func pullResolvedImage(ref string, img *Image, cacheDir string) (string, string, error) {
	cached := cachePath(cacheDir, img)

	expected, err := expectedHash(img)
	if err != nil {
		return "", "", fmt.Errorf("resolve checksum for %s: %w", ref, err)
	}

	if _, err := os.Stat(cached); err == nil {
		if cachedImageShouldBeVerified(expected) {
			if err := verifyFile(cached, expected); err != nil {
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
		// The downloader cleans up its own partial file. A concurrent pull
		// may have verified and promoted this destination in the meantime.
		return "", "", fmt.Errorf("pull %s: %w", ref, err)
	}
	fmt.Printf("cached  %s\n", cached)
	return cached, img.Format, nil
}

func cachedImageShouldBeVerified(expected imageHash) bool {
	return expected.Algorithm != ""
}

func ensureImageCacheDir(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, imageCacheDirPerm); err != nil {
		return err
	}
	// MkdirAll leaves an existing directory's mode untouched. Tighten caches
	// created by older releases so image contents remain private even when a
	// caller places the cache outside the already-private holos state tree.
	return os.Chmod(cacheDir, imageCacheDirPerm)
}
