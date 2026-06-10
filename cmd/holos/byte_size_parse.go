package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	byteSizeExample = "10GB"
	minByteSize     = int64(1 << 20)
)

func parseByteSize(raw string) (int64, error) {
	s, err := normalizeByteSize(raw)
	if err != nil {
		return 0, err
	}
	if s == "" {
		return 0, fmt.Errorf("empty size value")
	}

	multiplier := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'K':
		multiplier = 1 << 10
	case 'M':
		multiplier = 1 << 20
	case 'G':
		multiplier = 1 << 30
	case 'T':
		multiplier = 1 << 40
	}
	if multiplier != 1 {
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q (expected e.g. %q): %w", raw, byteSizeExample, err)
	}
	bytes := int64(value * float64(multiplier))
	if bytes < minByteSize {
		return 0, fmt.Errorf("size %q is below minimum %d bytes", raw, minByteSize)
	}
	return bytes, nil
}

func normalizeByteSize(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return "", nil
	}
	if !strings.HasSuffix(s, "B") {
		return s, nil
	}
	s = strings.TrimSuffix(s, "B")
	if s == "" {
		return "", fmt.Errorf("invalid size %q (expected e.g. %q)", raw, byteSizeExample)
	}
	return s, nil
}
