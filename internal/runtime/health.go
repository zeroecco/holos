package runtime

import (
	"context"
	"fmt"
	"time"
)

// probeFunc matches the probeHealthcheck signature so tests can
// substitute a deterministic prober without opening real SSH sessions.
type probeFunc func(ctx context.Context, timeout time.Duration) error

type healthTiming struct {
	interval    time.Duration
	startPeriod time.Duration
	timeout     time.Duration
}

// waitForHealthy polls probeHealthcheck in two distinct phases so
// `start_period` behaves the way its docstring promises:
//
//  1. Grace phase: for `start_period` wall-clock seconds, probe at
//     `interval` cadence. Any success returns early; failures are
//     ignored and do not count toward `retries`. This is the window
//     where slow guests (debian's cloud-init, kernel modprobe, ...)
//     can fail probes without consuming anybody's budget.
//
//  2. Retry phase: after grace ends, probe up to `retries` times at
//     `interval` cadence. The first success returns; after `retries`
//     consecutive failures the service is declared unhealthy.
//
// The previous max(start_period, retries*interval) budget conflated
// the two phases, so a long start_period (e.g. 60s, interval 10s,
// retries 3) would hit the deadline at 60s without ever granting the
// three post-grace retries the operator asked for.
func waitForHealthy(ctx context.Context, addr, user, keyPath string, cmd []string, intervalSec, retries, startPeriodSec, timeoutSec int) error {
	probe := func(ctx context.Context, timeout time.Duration) error {
		return probeHealthcheck(ctx, addr, user, keyPath, cmd, timeout)
	}
	timing := healthTimingFromSeconds(intervalSec, startPeriodSec, timeoutSec)
	return waitForHealthyWith(ctx, probe, timing.interval, retries, timing.startPeriod, timing.timeout)
}

// waitForHealthyWith is the phase-split loop factored out so unit
// tests can supply a fake probe and verify the retry accounting
// without touching ssh.
func waitForHealthyWith(ctx context.Context, probe probeFunc, interval time.Duration, retries int, startPeriod, timeout time.Duration) error {
	// Phase 1: grace window. Failures are tolerated; any success
	// short-circuits. We cap the sleep so the final attempt happens
	// right at the deadline rather than overshooting by up to
	// `interval`.
	if startPeriod > 0 {
		graceDeadline := time.Now().Add(startPeriod)
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := probe(ctx, timeout); err == nil {
				return nil
			}
			remaining := time.Until(graceDeadline)
			if remaining <= 0 {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(healthGraceSleep(interval, remaining)):
			}
		}
	}

	// Phase 2: post-grace retries. Each failed probe burns one slot.
	// `retries` is the number of attempts, not the number of failures
	// tolerated before a final try, so we loop up to `retries` total
	// and sleep between attempts (but not after the last one).
	if retries < 1 {
		retries = 1
	}
	var lastErr error
	for i := 0; i < retries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := probe(ctx, timeout); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i == retries-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return healthcheckRetriesError(retries, startPeriod, lastErr)
}

func healthcheckRetriesError(retries int, startPeriod time.Duration, err error) error {
	return fmt.Errorf("healthcheck failed after %d retries (start_period %s): %w", retries, startPeriod, err)
}

func healthGraceSleep(interval, remaining time.Duration) time.Duration {
	if interval > remaining {
		return remaining
	}
	return interval
}

func healthTimingFromSeconds(intervalSec, startPeriodSec, timeoutSec int) healthTiming {
	return healthTiming{
		interval:    secondsDuration(intervalSec),
		startPeriod: secondsDuration(startPeriodSec),
		timeout:     secondsDuration(timeoutSec),
	}
}

func secondsDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}
