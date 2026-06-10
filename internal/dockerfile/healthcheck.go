package dockerfile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zeroecco/holos/internal/config"
)

const (
	healthcheckCommand      = "CMD"
	healthcheckNone         = "NONE"
	healthcheckShellCommand = "sh"
	healthcheckShellFlag    = "-c"
)

func parseHealthcheck(args string) (*config.HealthcheckConfig, error) {
	args = strings.TrimSpace(args)
	if strings.EqualFold(args, healthcheckNone) {
		return nil, nil
	}

	healthcheck := &config.HealthcheckConfig{
		IntervalSec:      config.DefaultHealthIntervalSec,
		Retries:          config.DefaultHealthRetries,
		StartIntervalSec: config.DefaultHealthIntervalSec,
		TimeoutSec:       config.DefaultHealthTimeoutSec,
	}

	fields := strings.Fields(args)
	startIntervalExplicit := false
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		name, value, err := splitHealthcheckOption(fields[0])
		if err != nil {
			return nil, err
		}
		if err := applyHealthcheckOption(healthcheck, name, value); err != nil {
			return nil, err
		}
		if name == "--start-interval" {
			startIntervalExplicit = true
		}
		fields = fields[1:]
	}
	if !startIntervalExplicit {
		healthcheck.StartIntervalSec = healthcheck.IntervalSec
	}

	if len(fields) == 0 || !strings.EqualFold(fields[0], healthcheckCommand) {
		return nil, fmt.Errorf("HEALTHCHECK requires CMD or NONE")
	}
	cmd := strings.TrimSpace(strings.TrimPrefix(argsAfterOptions(args), fields[0]))
	if cmd == "" {
		return nil, fmt.Errorf("HEALTHCHECK CMD requires a command")
	}
	healthcheck.Test = healthcheckCommandArgs(cmd)
	return healthcheck, nil
}

func splitHealthcheckOption(raw string) (string, string, error) {
	name, value, ok := strings.Cut(raw, "=")
	if !ok || value == "" {
		return "", "", fmt.Errorf("HEALTHCHECK option %q must use --name=value", raw)
	}
	return name, value, nil
}

func applyHealthcheckOption(healthcheck *config.HealthcheckConfig, name, value string) error {
	switch name {
	case "--interval":
		seconds, err := parseHealthcheckDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		healthcheck.IntervalSec = seconds
	case "--timeout":
		seconds, err := parseHealthcheckDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		healthcheck.TimeoutSec = seconds
	case "--start-period":
		seconds, err := parseHealthcheckDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		healthcheck.StartPeriodSec = seconds
	case "--start-interval":
		seconds, err := parseHealthcheckDuration(value)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		healthcheck.StartIntervalSec = seconds
	case "--retries":
		retries, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("--retries: %w", err)
		}
		if retries < 1 {
			return fmt.Errorf("--retries must be >= 1")
		}
		healthcheck.Retries = retries
	default:
		return fmt.Errorf("unsupported HEALTHCHECK option %q", name)
	}
	return nil
}

func parseHealthcheckDuration(raw string) (int, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %q must be non-negative", raw)
	}
	seconds := int(d.Seconds())
	if d > 0 && seconds < 1 {
		return 1, nil
	}
	return seconds, nil
}

func healthcheckCommandArgs(args string) []string {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "[") {
		var parts []string
		if err := json.Unmarshal([]byte(args), &parts); err == nil {
			return parts
		}
	}
	return []string{healthcheckShellCommand, healthcheckShellFlag, args}
}

func argsAfterOptions(args string) string {
	remaining := strings.TrimSpace(args)
	for strings.HasPrefix(remaining, "--") {
		_, rest, ok := strings.Cut(remaining, " ")
		if !ok {
			return ""
		}
		remaining = strings.TrimSpace(rest)
	}
	return remaining
}
