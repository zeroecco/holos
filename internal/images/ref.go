package images

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	qcow2Extension = ".qcow2"
	rawExtension   = ".raw"
	imgExtension   = ".img"

	cacheFilenameStemFormat   = "%s-%s-%s"
	cacheFilenameURLHashBytes = 4

	refTagSeparator      = ":"
	absolutePathPrefix   = "/"
	currentDirPathPrefix = "./"
	parentDirPathPrefix  = "../"
)

var localImageExtensions = []string{qcow2Extension, rawExtension, imgExtension}

func cacheFilename(img *Image) string {
	return cacheFilenameStem(img) + cacheFilenameExtension(img)
}

func cacheFilenameStem(img *Image) string {
	return fmt.Sprintf(cacheFilenameStemFormat, img.Name, img.Tag, cacheFilenameURLHash(img.URL))
}

func cacheFilenameURLHash(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:cacheFilenameURLHashBytes])
}

func cacheFilenameExtension(img *Image) string {
	if img.Format == config.ImageFormatRaw {
		return rawExtension
	}
	return qcow2Extension
}

func cachePath(cacheDir string, img *Image) string {
	return filepath.Join(cacheDir, cacheFilename(img))
}

func parseRef(ref string) (name, tag string) {
	if idx := strings.LastIndex(ref, refTagSeparator); idx != -1 {
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
	if strings.HasPrefix(ref, absolutePathPrefix) ||
		strings.HasPrefix(ref, currentDirPathPrefix) ||
		strings.HasPrefix(ref, parentDirPathPrefix) {
		return true
	}
	if strings.Contains(ref, refTagSeparator) {
		return false
	}
	for _, ext := range localImageExtensions {
		if strings.HasSuffix(ref, ext) {
			return true
		}
	}
	return false
}

func inferFormat(path string) string {
	if isRawImagePath(path) {
		return config.ImageFormatRaw
	}
	return config.ImageFormatQCOW2
}

func isRawImagePath(path string) bool {
	return filepath.Ext(path) == rawExtension
}
