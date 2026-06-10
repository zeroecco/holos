package compose

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	volumeSizeBytesSuffix = "B"
	volumeSizeExample     = "10GB"

	volumeSizeUnitKiB = 'K'
	volumeSizeUnitMiB = 'M'
	volumeSizeUnitGiB = 'G'
	volumeSizeUnitTiB = 'T'
)

// parseVolumeSize accepts a human-friendly size string (case-insensitive):
// plain bytes ("1048576"), or a decimal with a unit suffix: K/M/G/T with an
// optional B ("2G", "2GB"). Multipliers are binary, matching qemu-img
// convention. Empty returns the default.
func parseVolumeSize(raw string) (int64, error) {
	if raw == "" {
		return defaultVolumeSizeBytes, nil
	}

	s, err := normalizeVolumeSize(raw)
	if err != nil {
		return 0, err
	}
	if s == "" {
		return defaultVolumeSizeBytes, nil
	}

	multiplier := int64(1)
	last := s[len(s)-1]
	multiplier = volumeSizeMultiplier(last)
	if multiplier != 1 {
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q (expected e.g. %q): %w", raw, volumeSizeExample, err)
	}
	bytes := int64(value * float64(multiplier))
	if bytes < minVolumeSizeBytes {
		return 0, fmt.Errorf("volume size %q is below minimum %d bytes", raw, minVolumeSizeBytes)
	}
	return bytes, nil
}

func normalizeVolumeSize(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return "", nil
	}
	if !strings.HasSuffix(s, volumeSizeBytesSuffix) {
		return s, nil
	}
	s = strings.TrimSuffix(s, volumeSizeBytesSuffix)
	if s == "" {
		return "", fmt.Errorf("invalid size %q (expected e.g. %q)", raw, volumeSizeExample)
	}
	return s, nil
}

func volumeSizeMultiplier(unit byte) int64 {
	switch unit {
	case volumeSizeUnitKiB:
		return 1 << 10
	case volumeSizeUnitMiB:
		return 1 << 20
	case volumeSizeUnitGiB:
		return 1 << 30
	case volumeSizeUnitTiB:
		return 1 << 40
	default:
		return 1
	}
}

const (
	// defaultVolumeSizeBytes is the virtual size used when a named
	// volume omits an explicit `size:` field. Matches docker's "what
	// you'd get if you didn't think about it" convention.
	defaultVolumeSizeBytes = 10 * (1 << 30) // 10 GiB

	// minVolumeSizeBytes is a sanity floor; below this qemu-img
	// rounding produces surprising results and most filesystems can't
	// even hold their own superblock.
	minVolumeSizeBytes = 1 * (1 << 20) // 1 MiB
)
