package runtime

import (
	"context"
	"fmt"
	"os"
	"time"
)

// probeBypassEnv lets tests (and environments without real VMs) short-
// circuit the ssh probe. When set to a truthy value, every healthcheck
// returns success immediately; the service's wait-for-healthy loop
// still runs so ordering is exercised, but no network dial is attempted.
const probeBypassEnv = "HOLOS_HEALTH_BYPASS"

// probeHealthcheck connects to an instance's sshd and runs the
// configured healthcheck command. A zero exit code is healthy; any
// other outcome (non-zero exit, dial failure, session error, timeout)
// surfaces as an error so the caller can count it against `retries`.
//
// The ssh connection and every session operation share a single
// deadline derived from timeout so a hung guest can't keep the probe
// alive forever.
func probeHealthcheck(ctx context.Context, addr, user, keyPath string, cmd []string, timeout time.Duration) error {
	if os.Getenv(probeBypassEnv) != "" {
		return nil
	}
	if len(cmd) == 0 {
		return fmt.Errorf("empty healthcheck command")
	}

	key, err := loadSSHKey(keyPath)
	if err != nil {
		return err
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := newHealthSSHClient(probeCtx, addr, user, key, timeout)
	if err != nil {
		return err
	}
	defer client.Close()

	return runHealthSSHCommand(client, cmd)
}
