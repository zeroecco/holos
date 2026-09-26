package images

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The selection is separate from the artifact: changing an alias must never
// overwrite a different build that an existing qcow2 overlay still uses.
type ubuntuSelection struct {
	Version   int    `json:"version"`
	SourceURL string `json:"source_url"`
	Release   string `json:"release"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
}

func ubuntuSelectionPath(cacheDir string, img *Image) string {
	return filepath.Join(cacheDir, cacheFilenameStem(img)+".json")
}

func ubuntuImageForPull(img *Image, cacheDir string) (*Image, bool, error) {
	selected, err := cachedUbuntuSelection(cacheDir, img)
	if err != nil || selected != nil {
		return selected, false, err
	}

	selected, err = resolveUbuntuImage(img)
	return selected, true, err
}

func cachedUbuntuSelection(cacheDir string, img *Image) (*Image, error) {
	selected, err := loadUbuntuSelection(cacheDir, img)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cachePath(cacheDir, selected)); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return selected, nil
}

func loadUbuntuSelection(cacheDir string, img *Image) (*Image, error) {
	path := ubuntuSelectionPath(cacheDir, img)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var selection ubuntuSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return nil, fmt.Errorf("decode image selection %s: %w", path, err)
	}
	if selection.Version != 1 || selection.SourceURL != img.URL || selection.Release != img.UbuntuRelease {
		return nil, fmt.Errorf("image selection %s does not match registry source", path)
	}
	if len(selection.SHA256) != sha256HexLength || !isHex(selection.SHA256) {
		return nil, fmt.Errorf("image selection %s has invalid SHA-256", path)
	}
	if err := validateUbuntuArtifactURL(img.URL, img.UbuntuRelease, selection.URL); err != nil {
		return nil, fmt.Errorf("image selection %s: %w", path, err)
	}
	resolved := *img
	resolved.URL = selection.URL
	resolved.SHA256 = strings.ToLower(selection.SHA256)
	resolved.SHA512, resolved.SHA256URL, resolved.SHA512URL = "", "", ""
	return &resolved, nil
}

// Downloads run concurrently, but the first successfully committed selection
// wins until its artifact is removed. Recheck under a process-shared lock so a
// delayed pull cannot replace a build another pull has already returned.
func commitUbuntuSelection(cacheDir string, entry, resolved *Image) (*Image, error) {
	lockPath := ubuntuSelectionPath(cacheDir, entry) + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open image selection lock: %w", err)
	}
	// Keep the lock file: unlinking it would let contenders lock different inodes.
	defer file.Close()
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("lock image selection: %w", err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	selected, err := cachedUbuntuSelection(cacheDir, entry)
	if err != nil || selected != nil {
		return selected, err
	}
	if err := saveUbuntuSelection(cacheDir, entry, resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

// Save only after the artifact is verified and promoted. An interrupted pull
// can leave an unreferenced image, but cannot pin an incomplete download.
func saveUbuntuSelection(cacheDir string, entry, resolved *Image) error {
	selection := ubuntuSelection{
		Version:   1,
		SourceURL: entry.URL,
		Release:   entry.UbuntuRelease,
		URL:       resolved.URL,
		SHA256:    resolved.SHA256,
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return err
	}
	path := ubuntuSelectionPath(cacheDir, entry)
	file, err := os.CreateTemp(cacheDir, ".ubuntu-selection-*")
	if err != nil {
		return fmt.Errorf("create image selection: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write image selection: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync image selection: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close image selection: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("save image selection: %w", err)
	}
	return nil
}
