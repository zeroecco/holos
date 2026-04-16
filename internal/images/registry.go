package images

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Image describes a downloadable cloud image.
type Image struct {
	Name    string // short name (e.g. "alpine")
	URL     string
	Format  string // qcow2 or raw
	Default bool   // true = default tag for this distro
	Tag     string // version tag (e.g. "3.21", "noble")
}

// Registry maps short names like "alpine" or "ubuntu:noble" to download URLs.
var Registry = []Image{
	// Alpine Linux (tiny-cloud, NoCloud datasource, BIOS)
	{Name: "alpine", Tag: "3.21", URL: "https://dl-cdn.alpinelinux.org/alpine/v3.21/releases/cloud/nocloud_alpine-3.21.6-x86_64-bios-tiny-r0.qcow2", Format: "qcow2", Default: true},

	// Arch Linux (cloud-init, official arch-boxes)
	{Name: "arch", Tag: "latest", URL: "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2", Format: "qcow2", Default: true},

	// Debian (NoCloud variant, cloud-init)
	{Name: "debian", Tag: "12", URL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-nocloud-amd64.qcow2", Format: "qcow2", Default: true},
	{Name: "debian", Tag: "bookworm", URL: "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-nocloud-amd64.qcow2", Format: "qcow2"},

	// Ubuntu (cloud images, NoCloud compatible)
	{Name: "ubuntu", Tag: "noble", URL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img", Format: "qcow2", Default: true},
	{Name: "ubuntu", Tag: "24.04", URL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img", Format: "qcow2"},
	{Name: "ubuntu", Tag: "jammy", URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img", Format: "qcow2"},
	{Name: "ubuntu", Tag: "22.04", URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img", Format: "qcow2"},

	// Fedora Cloud Base
	{Name: "fedora", Tag: "43", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/43/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-43-1.6.x86_64.qcow2", Format: "qcow2", Default: true},
}

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
		if tag == "" && img.Default {
			return img, nil
		}
		if tag != "" && img.Tag == tag {
			return img, nil
		}
	}

	if tag != "" {
		return nil, fmt.Errorf("unknown image %q (tag %q); run 'holos images' to list available images", name, tag)
	}
	return nil, fmt.Errorf("unknown image %q; run 'holos images' to list available images", name)
}

// Pull downloads an image to the cache directory, returning the local path.
// If already cached, returns immediately.
func Pull(ref string, cacheDir string) (localPath string, format string, err error) {
	img, err := Resolve(ref)
	if err != nil {
		return "", "", err
	}

	if img == nil {
		return ref, inferFormat(ref), nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create image cache: %w", err)
	}

	filename := cacheFilename(img)
	cached := filepath.Join(cacheDir, filename)

	if _, err := os.Stat(cached); err == nil {
		return cached, img.Format, nil
	}

	fmt.Printf("pulling %s:%s ...\n", img.Name, img.Tag)

	if err := download(img.URL, cached); err != nil {
		_ = os.Remove(cached)
		return "", "", fmt.Errorf("pull %s: %w", ref, err)
	}

	fmt.Printf("cached  %s\n", cached)
	return cached, img.Format, nil
}

// DefaultCacheDir returns the image cache directory.
func DefaultCacheDir(stateDir string) string {
	return filepath.Join(stateDir, "images")
}

// ListAvailable returns all registered images.
func ListAvailable() []Image {
	return Registry
}

func download(url, dest string) error {
	tmp := dest + ".part"

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	file, err := os.Create(tmp)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)

	size, err := io.Copy(writer, resp.Body)
	if err != nil {
		file.Close()
		_ = os.Remove(tmp)
		return err
	}
	file.Close()

	fmt.Printf("  %s  %d MB  sha256:%s\n",
		filepath.Base(dest),
		size/(1024*1024),
		hex.EncodeToString(hasher.Sum(nil))[:16],
	)

	return os.Rename(tmp, dest)
}

func cacheFilename(img *Image) string {
	h := sha256.Sum256([]byte(img.URL))
	short := hex.EncodeToString(h[:4])
	ext := ".qcow2"
	if img.Format == "raw" {
		ext = ".raw"
	}
	return fmt.Sprintf("%s-%s-%s%s", img.Name, img.Tag, short, ext)
}

func parseRef(ref string) (name, tag string) {
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

// isLocalPath decides whether a reference should be treated as a filesystem
// path rather than a registry name. We accept:
//
//   - Absolute paths ("/...")
//   - Relative paths explicitly rooted at "./" or "../"
//   - Bare filenames ending in .qcow2/.raw/.img, but only if they contain
//     no colon (so registry references like "ubuntu:noble" are never
//     misinterpreted even if a future tag happened to end in ".img")
func isLocalPath(ref string) bool {
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
		return true
	}
	if strings.ContainsRune(ref, ':') {
		return false
	}
	if strings.HasSuffix(ref, ".qcow2") || strings.HasSuffix(ref, ".raw") || strings.HasSuffix(ref, ".img") {
		return true
	}
	return false
}

func inferFormat(path string) string {
	switch filepath.Ext(path) {
	case ".raw":
		return "raw"
	default:
		return "qcow2"
	}
}
