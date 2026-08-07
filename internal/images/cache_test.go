package images

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCacheDir(t *testing.T) {
	t.Parallel()

	stateDir := filepath.FromSlash("state/holos")
	if got, want := DefaultCacheDir(stateDir), filepath.FromSlash("state/holos/images"); got != want {
		t.Fatalf("DefaultCacheDir = %q, want %q", got, want)
	}
}

func TestEnsureImageCacheDirMode(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "images")
	if err := ensureImageCacheDir(cacheDir); err != nil {
		t.Fatalf("ensureImageCacheDir: %v", err)
	}

	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}
	if got := info.Mode().Perm(); got != imageCacheDirPerm {
		t.Fatalf("cache dir mode = %v, want %v", got, imageCacheDirPerm)
	}
}

func TestEnsureImageCacheDirTightensExistingDirectory(t *testing.T) {
	t.Parallel()

	cacheDir := filepath.Join(t.TempDir(), "images")
	if err := os.Mkdir(cacheDir, 0o755); err != nil {
		t.Fatalf("create permissive cache dir: %v", err)
	}
	if err := ensureImageCacheDir(cacheDir); err != nil {
		t.Fatalf("ensureImageCacheDir: %v", err)
	}
	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}
	if got := info.Mode().Perm(); got != imageCacheDirPerm {
		t.Fatalf("cache mode = %#o, want %#o", got, imageCacheDirPerm)
	}
}
