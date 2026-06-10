package main

import (
	"fmt"
	"strconv"
	"strings"
)

func parseMemoryMB(raw string) (int, error) {
	s, multiplierMB, err := normalizeMemoryMBInput(raw)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(s) == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q: %w", raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("memory %q must be positive", raw)
	}
	mb := int(value * multiplierMB)
	if mb < 1 {
		return 0, fmt.Errorf("memory %q rounds to less than 1 MB", raw)
	}
	return mb, nil
}

func normalizeMemoryMBInput(raw string) (value string, multiplierMB float64, err error) {
	value = strings.TrimSpace(strings.ToUpper(raw))
	if value == "" {
		return "", 1, nil
	}

	multiplierMB = 1
	s := value
	last := s[len(s)-1]
	switch last {
	case 'B':
		if len(s) < 2 {
			return "", 0, fmt.Errorf("invalid memory %q", raw)
		}
		s = s[:len(s)-1]
		last = s[len(s)-1]
		fallthrough
	case 'K', 'M', 'G', 'T':
		switch last {
		case 'K':
			multiplierMB = 1.0 / 1024.0
		case 'M':
			multiplierMB = 1
		case 'G':
			multiplierMB = 1024
		case 'T':
			multiplierMB = 1024 * 1024
		}
		s = s[:len(s)-1]
	}
	return s, multiplierMB, nil
}
