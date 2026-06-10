package runtime

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/config"
)

const (
	// qmpHandshakeTimeout bounds how long we wait for the capability
	// negotiation and the system_powerdown ACK before abandoning QMP.
	// The ACK is expected to return promptly; the guest itself may take
	// far longer to actually halt.
	qmpHandshakeTimeout = 2 * time.Second

	// sigtermGrace is the window we give a process to exit after SIGTERM
	// before escalating to SIGKILL. This applies both as a fallback after
	// a failed QMP attempt and as a safety net for VMs that don't honour
	// ACPI powerdown.
	sigtermGrace = 10 * time.Second

	// waitForExitPollInterval is short enough for responsive shutdowns but
	// long enough to avoid a hot polling loop while waiting on QEMU.
	waitForExitPollInterval = 250 * time.Millisecond
)

// stopInstance requests a graceful shutdown, escalating as needed:
//
//  1. Send QMP system_powerdown and wait up to StopGracePeriodSec for the
//     guest to halt (ACPI shutdown, which lets the guest flush disks, unmount,
//     run shutdown units).
//  2. If the guest is still running after the grace period (or QMP was
//     unreachable), send SIGTERM to the qemu process and wait briefly.
//  3. If the process still hasn't exited, SIGKILL it.
//
// Returning nil means the process is no longer alive. A non-nil return
// from the signal sends is propagated so callers can surface kill errors,
// but any partial progress (QMP ACK, successful SIGTERM) is not rolled
// back.
func (m *Manager) stopInstance(instance InstanceRecord) error {
	if instance.PID == 0 || !processAlive(instance.PID) {
		return nil
	}

	if qmpPowerdownStopsInstance(instance) {
		return nil
	}

	process, err := os.FindProcess(instance.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", instance.PID, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal pid %d: %w", instance.PID, err)
	}
	if waitForExit(instance.PID, sigtermGrace) {
		return nil
	}

	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill pid %d: %w", instance.PID, err)
	}
	return nil
}

func qmpPowerdownStopsInstance(instance InstanceRecord) bool {
	if instance.QMPPath == "" {
		return false
	}
	if !requestPowerdown(instance.QMPPath) {
		return false
	}
	return waitForExit(instance.PID, stopGraceDuration(instance.StopGracePeriodSec))
}

// waitForExit polls processAlive at waitForExitPollInterval and returns true as
// soon as the process is no longer alive. Returns false on timeout.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(waitForExitPollInterval)
	}
	return !processAlive(pid)
}

func stopGraceDuration(stopGracePeriodSec int) time.Duration {
	if stopGracePeriodSec <= 0 {
		stopGracePeriodSec = config.DefaultStopGracePeriodSec
	}
	return secondsDuration(stopGracePeriodSec)
}

func (m *Manager) stopServiceInstances(project string, service *ServiceRecord) error {
	for idx := range service.Instances {
		if err := m.runPreStopCommands(project, *service, service.Instances[idx]); err != nil {
			return err
		}
		if err := m.stopInstance(service.Instances[idx]); err != nil {
			return err
		}
		m.cleanupInstanceTaps(service.Instances[idx])
		markInstanceStopped(&service.Instances[idx])
	}
	return nil
}

func (m *Manager) stopAllInstances(instances []InstanceRecord) {
	for idx := range instances {
		_ = m.stopInstance(instances[idx])
		m.cleanupInstanceTaps(instances[idx])
		markInstanceStopped(&instances[idx])
	}
}

func (m *Manager) removeInstances(instances []InstanceRecord) {
	m.stopAllInstances(instances)
	m.removeInstanceDirs(instances)
}

func markInstanceStopped(inst *InstanceRecord) {
	inst.Status = InstanceStatusStopped
	inst.PID = 0
	inst.LastExitTime = time.Now().UTC()
}

func (m *Manager) removeInstanceDirs(instances []InstanceRecord) {
	for _, inst := range instances {
		removeInstanceDir(inst)
	}
}

func removeInstanceDir(inst InstanceRecord) {
	if inst.WorkDir == "" {
		return
	}
	_ = os.RemoveAll(inst.WorkDir)
}
