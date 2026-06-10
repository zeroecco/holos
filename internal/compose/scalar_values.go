package compose

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func composeInt(value any, label string) (int, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("%s must be an integer", label)
		}
		return int(v), nil
	case string:
		return composeIntString(v, label)
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", label, value)
	}
}

func composeIntString(value string, label string) (int, error) {
	if isBlankScalarString(value) {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", label, err)
	}
	return parsed, nil
}

func composeFloat(value any, label string) (float64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return composeFloatString(v, label)
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", label, value)
	}
}

func composeFloatString(value string, label string) (float64, error) {
	if isBlankScalarString(value) {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", label, err)
	}
	return parsed, nil
}

func isBlankScalarString(value string) bool {
	return strings.TrimSpace(value) == ""
}
