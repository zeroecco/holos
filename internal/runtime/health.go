package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
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

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(key)},
		// Guest host keys change every `down`/`up`; pinning them
		// would force operators to manage known_hosts for ephemeral
		// fleets. Fingerprints still gain weak protection from the
		// fact that we only dial 127.0.0.1 on a port we just bound.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}
	rawConn, err := dialer.DialContext(probeCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	_ = rawConn.SetDeadline(time.Now().Add(timeout))

	clientConn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, config)
	if err != nil {
		_ = rawConn.Close()
		return fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(clientConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr
	session.Stdout = io.Discard

	shellCmd := shellJoin(cmd)
	if err := session.Run(shellCmd); err != nil {
		if exit, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("healthcheck exit=%d: %s", exit.ExitStatus(), stderr.String())
		}
		return fmt.Errorf("healthcheck run: %w", err)
	}
	return nil
}

// probeFunc matches the probeHealthcheck signature so tests can
// substitute a deterministic prober without opening real SSH sessions.
type probeFunc func(ctx context.Context, timeout time.Duration) error

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
	return waitForHealthyWith(ctx, probe,
		time.Duration(intervalSec)*time.Second,
		retries,
		time.Duration(startPeriodSec)*time.Second,
		time.Duration(timeoutSec)*time.Second,
	)
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
			wait := interval
			if wait > remaining {
				wait = remaining
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
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
	return fmt.Errorf("healthcheck failed after %d retries (start_period %s): %w", retries, startPeriod, lastErr)
}

// loadSSHKey parses the ed25519 OpenSSH private key at path.
func loadSSHKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}
	return signer, nil
}

// shellJoin renders argv as a single string suitable for
// session.Run (which hands the whole string to the remote shell),
// quoting each element defensively so arguments containing spaces or
// shell metacharacters survive the round trip.
func shellJoin(argv []string) string {
	buf := make([]byte, 0, 64)
	for i, a := range argv {
		if i > 0 {
			buf = append(buf, ' ')
		}
		buf = append(buf, '\'')
		for _, r := range a {
			if r == '\'' {
				buf = append(buf, '\'', '\\', '\'', '\'')
			} else {
				buf = append(buf, string(r)...)
			}
		}
		buf = append(buf, '\'')
	}
	return string(buf)
}
