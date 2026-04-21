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

// waitForHealthy polls probeHealthcheck until either it returns nil
// (healthy) or the budget is exhausted. The budget is the max of
// start_period and retries*interval so users can set either knob to
// control the overall deadline.
//
// During the start_period window failures are ignored entirely so a
// slow-booting service doesn't burn its retry budget on attempts that
// happen before sshd is even listening.
func waitForHealthy(ctx context.Context, addr, user, keyPath string, cmd []string, intervalSec, retries, startPeriodSec, timeoutSec int) error {
	interval := time.Duration(intervalSec) * time.Second
	timeout := time.Duration(timeoutSec) * time.Second
	startPeriod := time.Duration(startPeriodSec) * time.Second
	budget := time.Duration(retries) * interval
	if startPeriod > budget {
		budget = startPeriod
	}

	deadline := time.Now().Add(budget)
	start := time.Now()

	for attempt := 0; ; attempt++ {
		if err := probeHealthcheck(ctx, addr, user, keyPath, cmd, timeout); err == nil {
			return nil
		} else if time.Since(start) >= startPeriod && attempt >= retries-1 && time.Now().After(deadline) {
			return fmt.Errorf("healthcheck never succeeded within %s: %w", budget, err)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("healthcheck budget %s exhausted", budget)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
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
