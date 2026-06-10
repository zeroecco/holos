package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	doctorProbeTimeout        = 3 * time.Second
	doctorProbeTimeoutMessage = "probe timed out"
	doctorProbeLineSeparator  = '\n'
)

func runDoctorProbe(path string, args []string) (string, error) {
	return runDoctorProbeWithTimeout(path, args, doctorProbeTimeout)
}

func runDoctorProbeWithTimeout(path string, args []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf(doctorProbeTimeoutMessage)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return "", err
	}
	return firstLine(strings.TrimSpace(string(out))), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, doctorProbeLineSeparator); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
