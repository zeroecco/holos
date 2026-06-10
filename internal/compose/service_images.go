package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/images"
)

type composeImageResolver interface {
	Pull(ref string, cacheDir string) (path string, format string, err error)
	OSFamily(ref string) string
	MinMemoryMB(ref string) int
	DefaultUser(ref string) string
	RequiresVGA(ref string) bool
}

type registryImageResolver struct{}

func (registryImageResolver) Pull(ref string, cacheDir string) (string, string, error) {
	return images.Pull(ref, cacheDir)
}

func (registryImageResolver) OSFamily(ref string) string {
	return images.OSFamily(ref)
}

func (registryImageResolver) MinMemoryMB(ref string) int {
	return images.MinMemoryMB(ref)
}

func (registryImageResolver) DefaultUser(ref string) string {
	return images.DefaultUser(ref)
}

func (registryImageResolver) RequiresVGA(ref string) bool {
	return images.RequiresVGA(ref)
}

func (f *File) resolver() composeImageResolver {
	if f.imageResolver != nil {
		return f.imageResolver
	}
	return registryImageResolver{}
}

func resolveImage(ref string, explicitFormat string, baseDir string, cacheDir string, resolver composeImageResolver) (path string, format string, err error) {
	path, format, err = resolver.Pull(ref, cacheDir)
	if err != nil {
		return "", "", err
	}

	path = resolveImagePath(baseDir, path)

	// Cached remote images are guaranteed to exist: images.Pull only
	// returns a cached path after stating it. Local-path refs
	// (images.Pull returns them verbatim when the ref is not a
	// registry entry) bypass that stat, so `holos validate` would
	// silently approve `image: ./missing.qcow2` and the real error
	// would not surface until qemu-img failed deep in `holos up`.
	// Checking here turns the silent pass into an early, specific
	// failure that names the compose field.
	if err := validateImagePath(ref, path); err != nil {
		return "", "", err
	}

	format = resolveImageFormat(format, explicitFormat)
	return path, format, nil
}

func resolveImageFormat(pulledFormat string, explicitFormat string) string {
	if explicitFormat != "" {
		return explicitFormat
	}
	return pulledFormat
}

func resolveImageOS(ref string, explicitOS string, resolver composeImageResolver) string {
	if explicitOS != "" {
		return explicitOS
	}
	if osFamily := resolver.OSFamily(ref); osFamily != "" {
		return osFamily
	}
	return config.ImageOSSystemd
}

func resolveImagePath(baseDir string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	path = filepath.Join(baseDir, path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path
}

func validateImagePath(ref string, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("image %q: %w", ref, err)
	}
	if info.IsDir() {
		return fmt.Errorf("image %q is a directory, expected a qcow2 or raw file", ref)
	}
	return nil
}
