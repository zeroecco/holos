package images

import "path/filepath"

const cacheStateSubdir = "images"

// DefaultCacheDir returns the image cache directory.
func DefaultCacheDir(stateDir string) string {
	return filepath.Join(stateDir, cacheStateSubdir)
}
