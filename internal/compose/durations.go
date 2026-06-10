package compose

import (
	"fmt"
	"time"
)

const defaultStopGracePeriodSec = 0

// parseDurationSec accepts a Go duration string and returns whole
// seconds, returning the fallback when the input is empty. Values
// below 1s round up to 1s so healthcheck loops never busy-spin on
// fractional intervals.
func parseDurationSec(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %q must be non-negative", raw)
	}
	return durationSecondsRoundedUp(d), nil
}

// parseStopGracePeriod accepts a Go duration string (e.g. "30s", "2m") and
// returns it as whole seconds. Empty string yields 0 so callers can apply
// their own default.
func parseStopGracePeriod(raw string) (int, error) {
	if raw == "" {
		return defaultStopGracePeriodSec, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("stop_grace_period %q: %w", raw, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("stop_grace_period %q: must be non-negative", raw)
	}
	return durationSecondsRoundedUp(d), nil
}

func durationSecondsRoundedUp(d time.Duration) int {
	seconds := int(d.Seconds())
	if d > 0 && seconds < 1 {
		return 1
	}
	return seconds
}
