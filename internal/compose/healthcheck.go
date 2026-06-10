package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Healthcheck declares a liveness probe for a service. When set,
// `holos up` blocks on every dependent until this service reports
// healthy, mirroring docker-compose's `depends_on: condition:
// service_healthy` without requiring the verbose map form.
//
// The probe is a shell command run inside each replica over the
// project's auto-generated `holos exec` ssh key. Exit 0 is healthy;
// any other exit or a transport error counts as an attempt failure.
type Healthcheck struct {
	// Test is the shell command to run inside the VM. Accepts either
	// a YAML list (["pg_isready"]) or a single string ("curl -f
	// http://localhost").
	Test []string `yaml:"test,omitempty"`

	// Interval between probe attempts (e.g. "10s"). Defaults to 30s.
	Interval string `yaml:"interval,omitempty"`
	// Retries is how many consecutive failures count as unhealthy
	// AFTER start_period has elapsed. Defaults to 3.
	Retries int `yaml:"retries,omitempty"`
	// StartPeriod is a grace window right after boot during which
	// failures do not count toward `retries`. Defaults to 0 (no grace).
	StartPeriod string `yaml:"start_period,omitempty"`
	// StartInterval sets the probe cadence during StartPeriod. When omitted,
	// Holos uses Interval for probing.
	StartInterval string `yaml:"start_interval,omitempty"`
	// Timeout bounds a single probe's wall-clock budget. Defaults
	// to 5s.
	Timeout string `yaml:"timeout,omitempty"`
	// Disable accepts Docker Compose's explicit healthcheck opt-out.
	Disable bool `yaml:"disable,omitempty"`
}

const (
	healthcheckFieldTest          = "test"
	healthcheckFieldInterval      = "interval"
	healthcheckFieldRetries       = "retries"
	healthcheckFieldStartPeriod   = "start_period"
	healthcheckFieldStartInterval = "start_interval"
	healthcheckFieldTimeout       = "timeout"
	healthcheckFieldDisable       = "disable"

	healthcheckShellCommand = "sh"
	healthcheckShellFlag    = "-c"
)

var healthcheckAllowedFields = map[string]struct{}{
	healthcheckFieldTest:          {},
	healthcheckFieldInterval:      {},
	healthcheckFieldRetries:       {},
	healthcheckFieldStartPeriod:   {},
	healthcheckFieldStartInterval: {},
	healthcheckFieldTimeout:       {},
	healthcheckFieldDisable:       {},
}

// UnmarshalYAML accepts Healthcheck.Test as either a list of strings
// (canonical docker-compose form) or a single shell string. The single-
// string form is wrapped in ["sh", "-c", ...] so it runs through the
// shell exactly like docker-compose's CMD-SHELL variant.
//
// The outer Load() uses yaml.Decoder.KnownFields(true), but that
// setting is lost as soon as a custom UnmarshalYAML takes over:
// yaml.Node.Decode has no equivalent toggle. To keep the same
// strict-typo guarantee ("retriez:" is an error, not a silently
// dropped field), we explicitly enumerate this struct's keys and
// reject anything else before calling node.Decode.
func (h *Healthcheck) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if !isHealthcheckAllowedField(key) {
				return fmt.Errorf("line %d: field %s not found in type compose.Healthcheck", node.Content[i].Line, key)
			}
		}
	}

	type rawHealthcheck struct {
		Test          yaml.Node `yaml:"test"`
		Interval      string    `yaml:"interval"`
		Retries       int       `yaml:"retries"`
		StartPeriod   string    `yaml:"start_period"`
		StartInterval string    `yaml:"start_interval"`
		Timeout       string    `yaml:"timeout"`
		Disable       bool      `yaml:"disable"`
	}
	var raw rawHealthcheck
	if err := node.Decode(&raw); err != nil {
		return err
	}

	h.Interval = raw.Interval
	h.Retries = raw.Retries
	h.StartPeriod = raw.StartPeriod
	h.StartInterval = raw.StartInterval
	h.Timeout = raw.Timeout
	h.Disable = raw.Disable

	switch raw.Test.Kind {
	case 0:
		// omitted
	case yaml.ScalarNode:
		var s string
		if err := raw.Test.Decode(&s); err != nil {
			return err
		}
		if s != "" {
			h.Test = []string{healthcheckShellCommand, healthcheckShellFlag, s}
		}
	case yaml.SequenceNode:
		var list []string
		if err := raw.Test.Decode(&list); err != nil {
			return err
		}
		h.Test = list
	default:
		return fmt.Errorf("healthcheck.test must be a string or list of strings")
	}
	return nil
}

func isHealthcheckAllowedField(key string) bool {
	_, ok := healthcheckAllowedFields[key]
	return ok
}
