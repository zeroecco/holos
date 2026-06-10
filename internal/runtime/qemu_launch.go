package runtime

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	qemuHostForwardMarker = "hostfwd"
	qemuStartupProbeDelay = time.Second
	qemuLogPerm           = os.FileMode(0o644)
)

var qemuHostPortConflictMessages = []string{
	"address already in use",
	"could not set up host forwarding",
	"failed to set up host forwarding",
}

func (m *Manager) launchWithPortRetry(manifest config.Manifest, base qemu.LaunchSpec, qemuLogPath string, nextSpec func() (qemu.LaunchSpec, error)) (int, error) {
	var lastErr error
	for attempt := 1; attempt <= portRetryAttempts; attempt++ {
		spec, err := nextSpec()
		if err != nil {
			return 0, err
		}
		args, err := qemu.BuildArgs(manifest, spec)
		if err != nil {
			return 0, err
		}
		pid, logText, err := m.launchQEMU(args, qemuLogPath, base.Name)
		if err == nil {
			return pid, nil
		}
		lastErr = err
		if !isQEMUHostPortConflict(logText) {
			return 0, err
		}
		fmt.Fprintf(os.Stderr, "warning: qemu reported a host port conflict for %s; retrying with fresh ephemeral ports (%d/%d)\n",
			base.Name, attempt, portRetryAttempts)
	}
	return 0, lastErr
}

func (m *Manager) launchQEMU(args []string, qemuLogPath, instanceName string) (pid int, logText string, err error) {
	qemuLog, err := os.OpenFile(qemuLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, qemuLogPerm)
	if err != nil {
		return 0, "", fmt.Errorf("open qemu log: %w", err)
	}
	defer qemuLog.Close()

	command, err := m.qemuSystemCommand(args...)
	if err != nil {
		return 0, "", err
	}
	command.Stdout = qemuLog
	command.Stderr = qemuLog
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := command.Start(); err != nil {
		return 0, "", fmt.Errorf("start qemu: %w", err)
	}

	pid = command.Process.Pid

	exited := make(chan error, 1)
	go func() {
		exited <- command.Wait()
	}()

	select {
	case <-exited:
		return qemuEarlyExit(qemuLogPath, instanceName)
	case <-time.After(qemuStartupProbeDelay):
		select {
		case <-exited:
			return qemuEarlyExit(qemuLogPath, instanceName)
		default:
		}
		return pid, "", nil
	}
}

func qemuEarlyExit(qemuLogPath, instanceName string) (pid int, logText string, err error) {
	content, _ := os.ReadFile(qemuLogPath)
	logText = strings.TrimSpace(string(content))
	return 0, logText, fmt.Errorf("qemu exited early for %s: %s", instanceName, logText)
}

func isQEMUHostPortConflict(logText string) bool {
	lower := strings.ToLower(logText)
	if !strings.Contains(lower, qemuHostForwardMarker) {
		return false
	}
	for _, message := range qemuHostPortConflictMessages {
		if strings.Contains(lower, message) {
			return true
		}
	}
	return false
}
