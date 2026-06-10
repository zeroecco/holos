package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

// resolveHealthcheck validates and normalises a compose healthcheck
// block into the resolved config form. Absent blocks pass through as
// nil so consumers never have to check zero-value fields.
func resolveHealthcheck(h *Healthcheck) (*config.HealthcheckConfig, error) {
	if h == nil {
		return nil, nil
	}
	if h.Disable || (len(h.Test) == 1 && h.Test[0] == "NONE") {
		return nil, nil
	}
	if len(h.Test) == 0 {
		return nil, fmt.Errorf("healthcheck.test is required")
	}
	intervalSec, err := parseDurationSec(h.Interval, config.DefaultHealthIntervalSec)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.interval: %w", err)
	}
	startSec, err := parseDurationSec(h.StartPeriod, 0)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.start_period: %w", err)
	}
	startIntervalSec, err := parseDurationSec(h.StartInterval, intervalSec)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.start_interval: %w", err)
	}
	timeoutSec, err := parseDurationSec(h.Timeout, config.DefaultHealthTimeoutSec)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.timeout: %w", err)
	}
	return healthcheckConfig(h, intervalSec, healthcheckRetries(h.Retries), startSec, startIntervalSec, timeoutSec), nil
}

func healthcheckRetries(retries int) int {
	if retries == 0 {
		return config.DefaultHealthRetries
	}
	return retries
}

func healthcheckConfig(h *Healthcheck, intervalSec, retries, startSec, startIntervalSec, timeoutSec int) *config.HealthcheckConfig {
	return &config.HealthcheckConfig{
		Test:             append([]string{}, h.Test...),
		IntervalSec:      intervalSec,
		Retries:          retries,
		StartPeriodSec:   startSec,
		StartIntervalSec: startIntervalSec,
		TimeoutSec:       timeoutSec,
	}
}
