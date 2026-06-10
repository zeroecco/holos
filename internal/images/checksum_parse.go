package images

import "strings"

const (
	checksumManifestLineSeparator = "\n"
	checksumFilenameTrimPrefix    = "*"
	checksumFilenameTrimCutset    = "()"
	checksumHashTrimCutset        = "()*="
)

func findChecksum(data, filename string, hexLen int) string {
	var onlyHash string
	ambiguous := false
	for _, line := range strings.Split(data, checksumManifestLineSeparator) {
		lineHash := checksumLineHash(line, hexLen)
		if lineHash == "" {
			continue
		}
		if onlyHash == "" {
			onlyHash = lineHash
		} else if onlyHash != lineHash {
			ambiguous = true
		}
		if checksumLineMatchesFilename(line, filename) {
			return lineHash
		}
	}
	if onlyHash != "" && !ambiguous {
		return onlyHash
	}
	return ""
}

func checksumLineMatchesFilename(line, filename string) bool {
	for _, field := range strings.Fields(line) {
		field = strings.TrimLeft(field, checksumFilenameTrimPrefix)
		field = strings.Trim(field, checksumFilenameTrimCutset)
		if field == filename {
			return true
		}
	}
	return false
}

func checksumLineHash(line string, hexLen int) string {
	for _, field := range strings.Fields(line) {
		field = strings.Trim(field, checksumHashTrimCutset)
		if len(field) == hexLen && isHex(field) {
			return strings.ToLower(field)
		}
	}
	return ""
}
