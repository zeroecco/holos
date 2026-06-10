package compose

import (
	"fmt"
	"strings"
)

const (
	portSpecOpenBracket  = '['
	portSpecCloseBracket = ']'
	portSpecSeparator    = ':'
)

const invalidPortSpecError = "invalid port spec"

func splitPortSpec(spec string) ([]string, error) {
	var parts []string
	var b strings.Builder
	inBrackets := false
	for _, r := range spec {
		switch r {
		case portSpecOpenBracket:
			if inBrackets {
				return nil, fmt.Errorf(invalidPortSpecError)
			}
			inBrackets = true
			b.WriteRune(r)
		case portSpecCloseBracket:
			if !inBrackets {
				return nil, fmt.Errorf(invalidPortSpecError)
			}
			inBrackets = false
			b.WriteRune(r)
		case portSpecSeparator:
			if inBrackets {
				b.WriteRune(r)
				continue
			}
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if inBrackets {
		return nil, fmt.Errorf(invalidPortSpecError)
	}
	parts = append(parts, b.String())
	return parts, nil
}
