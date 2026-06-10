package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	volumeSpecSeparator = ":"
	volumeModeReadOnly  = "ro"
	volumeModeReadWrite = "rw"
)

// parseVolume splits a compose-style volume spec ("source:target[:ro]")
// into a typed Mount. Sources that match a declared top-level volume are
// treated as named (block) volumes; everything else is a host bind mount
// (virtfs), preserving existing behavior.
func parseVolume(spec string, baseDir string, declared map[string]Volume) (config.Mount, error) {
	parts, err := splitVolumeSpec(spec)
	if err != nil {
		return config.Mount{}, err
	}

	source := parts[0]
	target := parts[1]
	readOnly, err := parseVolumeMode(spec, parts)
	if err != nil {
		return config.Mount{}, err
	}

	if vol, ok := declared[source]; ok {
		sizeBytes, err := parseVolumeSize(vol.Size)
		if err != nil {
			return config.Mount{}, fmt.Errorf("volume %q: %w", source, err)
		}
		return namedVolumeMount(source, target, sizeBytes, readOnly), nil
	}

	source, err = resolveBindVolumeSource(source, baseDir)
	if err != nil {
		return config.Mount{}, err
	}

	return bindVolumeMount(source, target, readOnly), nil
}

func bindVolumeMount(source, target string, readOnly bool) config.Mount {
	return config.Mount{
		Kind:     config.MountKindBind,
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}
}

func namedVolumeMount(name, target string, sizeBytes int64, readOnly bool) config.Mount {
	return config.Mount{
		Kind:       config.MountKindVolume,
		VolumeName: name,
		SizeBytes:  sizeBytes,
		Target:     target,
		ReadOnly:   readOnly,
	}
}

func splitVolumeSpec(spec string) ([]string, error) {
	parts := strings.SplitN(spec, volumeSpecSeparator, 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("volume requires source:target")
	}
	return parts, nil
}

func parseVolumeMode(spec string, parts []string) (bool, error) {
	if len(parts) < 3 {
		return false, nil
	}

	// Only `ro` is supported today. The previous implementation
	// accepted anything here and silently fell back to read-write,
	// so a typo like `:readonly`, `:r0`, or docker-compose's
	// `:rw,Z` got interpreted as "mount it writable" and the
	// operator had no signal that their intent was dropped. Fail
	// loudly instead; the day we grow more modes (e.g. rshared,
	// noexec, nodev) we add them to this allow-list deliberately.
	switch parts[2] {
	case volumeModeReadOnly:
		return true, nil
	case volumeModeReadWrite:
		return false, nil
	default:
		return false, fmt.Errorf(
			"volume %q: unknown mode %q (supported: ro, rw)",
			spec, parts[2])
	}
}

func resolveBindVolumeSource(source, baseDir string) (string, error) {
	// Distinguish bind mounts from named volumes the same way docker
	// compose does: anything that looks like a path (absolute, ./,
	// ../, or containing a separator) is a bind mount; anything else
	// is a named-volume reference that must match a declared volume.
	// Treating a bare identifier as an implicit relative bind mount
	// would mask typos like `dta:/mnt`, so we reject it explicitly.
	if !looksLikePath(source) {
		return "", fmt.Errorf(
			"volume source %q is not a declared top-level volume and does not look like a path; "+
				"add it under volumes: or prefix with ./ for a bind mount",
			source)
	}

	return absoluteBindVolumeSource(source, baseDir), nil
}

func absoluteBindVolumeSource(source, baseDir string) string {
	if filepath.IsAbs(source) {
		return source
	}

	source = filepath.Join(baseDir, source)
	if abs, err := filepath.Abs(source); err == nil {
		return abs
	}
	return source
}

// looksLikePath returns true for strings a user would expect to be
// interpreted as filesystem paths: absolute paths, explicit ./ or ../
// roots, or anything containing a path separator. Bare identifiers
// ("data", "cache") are treated as named-volume references.
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	if filepath.IsAbs(s) {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	return strings.ContainsRune(s, os.PathSeparator)
}
