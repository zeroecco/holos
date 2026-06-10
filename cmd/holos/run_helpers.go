package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

var runNameSanitiser = regexp.MustCompile(`[^a-z0-9-]+`)

const (
	maxRunNameLength      = 63
	runNameSuffixBytes    = 3
	runNameSuffixLength   = 1 + runNameSuffixBytes*2
	runNameDockerfileBase = "dockerfile"
	runNameFallbackBase   = "vm"
	runNameSeparator      = "-"
)

func generateRunName(image string) string {
	base := image
	if base == "" {
		base = runNameDockerfileBase
	}
	suffix := randHex(runNameSuffixBytes)
	return runNameBase(base) + runNameSeparator + suffix
}

func runNameBase(base string) string {
	base = filepath.Base(base)
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		base = base[:dot]
	}
	base = strings.ToLower(base)
	base = runNameSanitiser.ReplaceAllString(base, runNameSeparator)
	base = strings.Trim(base, runNameSeparator)
	if base == "" {
		base = runNameFallbackBase
	}
	if len(base) > maxRunNameLength-runNameSuffixLength {
		base = base[:maxRunNameLength-runNameSuffixLength]
		base = strings.TrimRight(base, runNameSeparator)
	}
	return base
}
