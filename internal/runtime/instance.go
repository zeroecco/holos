package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
	"github.com/zeroecco/holos/internal/qmp"
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

	portRetryAttempts = 5
)

func (m *Manager) startInstance(project string, manifest config.Manifest, index int) (InstanceRecord, error) {
	workDir := projectInstanceDir(m.stateDir, project, manifest.Name, index)
	if err := os.RemoveAll(workDir); err != nil {
		return InstanceRecord{}, fmt.Errorf("remove instance workdir: %w", err)
	}
	// 0700 mirrors the rest of the state tree: this dir holds the
	// overlay qcow2, qmp socket, and console.log; nothing in there is
	// meant for other users on the host.
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return InstanceRecord{}, fmt.Errorf("create instance workdir: %w", err)
	}

	overlayPath := filepath.Join(workDir, "root.qcow2")
	if err := m.createOverlay(manifest, overlayPath); err != nil {
		return InstanceRecord{}, err
	}

	instanceName := manifest.InstanceName(index)

	seedPath, err := m.createSeedImage(manifest, instanceName, index, workDir)
	if err != nil {
		return InstanceRecord{}, err
	}

	logPath := filepath.Join(workDir, "console.log")
	serialPath := filepath.Join(workDir, "serial.sock")
	qmpPath := filepath.Join(workDir, "qmp.sock")
	qemuLogPath := filepath.Join(workDir, "qemu.log")

	volumes, err := materializeInstanceVolumes(m.stateDir, project, workDir, manifest.Mounts)
	if err != nil {
		return InstanceRecord{}, err
	}

	baseSpec := qemu.LaunchSpec{
		Name:        instanceName,
		Index:       index,
		OverlayPath: overlayPath,
		SeedPath:    seedPath,
		LogPath:     logPath,
		SerialPath:  serialPath,
		QMPPath:     qmpPath,
		Volumes:     volumes,
	}
	if manifest.VM.UEFI {
		ovmfCode, ovmfVars, err := m.prepareUEFI(workDir)
		if err != nil {
			return InstanceRecord{}, err
		}
		baseSpec.OVMFCode = ovmfCode
		baseSpec.OVMFVars = ovmfVars
	}

	var ports []qemu.PortMapping
	var sshPort int
	pid, err := m.launchWithPortRetry(manifest, baseSpec, qemuLogPath, func() (qemu.LaunchSpec, error) {
		var err error
		ports, err = allocatePorts(manifest, index)
		if err != nil {
			return qemu.LaunchSpec{}, err
		}
		sshPort, err = allocateEphemeralTCPPort()
		if err != nil {
			return qemu.LaunchSpec{}, fmt.Errorf("allocate ssh port: %w", err)
		}
		spec := baseSpec
		spec.Ports = ports
		spec.SSHPort = sshPort
		return spec, nil
	})
	if err != nil {
		return InstanceRecord{}, err
	}

	return InstanceRecord{
		Name:               instanceName,
		Index:              index,
		PID:                pid,
		Status:             "running",
		WorkDir:            workDir,
		OverlayPath:        overlayPath,
		SeedPath:           seedPath,
		LogPath:            logPath,
		SerialPath:         serialPath,
		QMPPath:            qmpPath,
		Ports:              ports,
		StopGracePeriodSec: manifest.StopGracePeriodSec,
		SSHPort:            sshPort,
		LastStarted:        time.Now().UTC(),
	}, nil
}

// restartInstance boots an existing stopped instance without recreating
// its overlay or seed image, preserving VM disk state across stop/start.
func (m *Manager) restartInstance(project string, manifest config.Manifest, prev InstanceRecord) (InstanceRecord, error) {
	qemuLogPath := filepath.Join(prev.WorkDir, "qemu.log")

	// Volume symlinks may have been removed manually or by a partial
	// cleanup; recreate them idempotently before boot.
	volumes, err := materializeInstanceVolumes(m.stateDir, project, prev.WorkDir, manifest.Mounts)
	if err != nil {
		return InstanceRecord{}, err
	}

	// Restarts try to keep the previously-issued ssh port so that an
	// operator's shell history (`ssh -p 51234 ...`) and any ambient
	// firewall rules keep working. If that port got grabbed by
	// another process between stop and start, fall back to a fresh
	// allocation rather than failing the boot.
	baseSpec := qemu.LaunchSpec{
		Name:        prev.Name,
		Index:       prev.Index,
		OverlayPath: prev.OverlayPath,
		SeedPath:    prev.SeedPath,
		LogPath:     prev.LogPath,
		SerialPath:  prev.SerialPath,
		QMPPath:     prev.QMPPath,
		Volumes:     volumes,
	}

	if manifest.VM.UEFI {
		firmware, err := ResolveOVMFFirmware()
		if err != nil {
			return InstanceRecord{}, err
		}
		baseSpec.OVMFCode = firmware.CodePath
		baseSpec.OVMFVars = filepath.Join(prev.WorkDir, "OVMF_VARS.fd")
	}

	var ports []qemu.PortMapping
	var sshPort int
	first := true
	pid, err := m.launchWithPortRetry(manifest, baseSpec, qemuLogPath, func() (qemu.LaunchSpec, error) {
		var err error
		ports, err = allocatePorts(manifest, prev.Index)
		if err != nil {
			return qemu.LaunchSpec{}, err
		}
		sshPort = prev.SSHPort
		if !first || sshPort == 0 || ensureTCPPortAvailable(sshPort) != nil {
			sshPort, err = allocateEphemeralTCPPort()
			if err != nil {
				return qemu.LaunchSpec{}, fmt.Errorf("allocate ssh port: %w", err)
			}
		}
		first = false
		spec := baseSpec
		spec.Ports = ports
		spec.SSHPort = sshPort
		return spec, nil
	})
	if err != nil {
		return InstanceRecord{}, err
	}

	return InstanceRecord{
		Name:               prev.Name,
		Index:              prev.Index,
		PID:                pid,
		Status:             "running",
		WorkDir:            prev.WorkDir,
		OverlayPath:        prev.OverlayPath,
		SeedPath:           prev.SeedPath,
		LogPath:            prev.LogPath,
		SerialPath:         prev.SerialPath,
		QMPPath:            prev.QMPPath,
		Ports:              ports,
		StopGracePeriodSec: manifest.StopGracePeriodSec,
		SSHPort:            sshPort,
		LastStarted:        time.Now().UTC(),
	}, nil
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
	qemuLog, err := os.OpenFile(qemuLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
	_ = command.Process.Release()

	time.Sleep(300 * time.Millisecond)
	if processAlive(pid) {
		return pid, "", nil
	}
	content, _ := os.ReadFile(qemuLogPath)
	logText = strings.TrimSpace(string(content))
	return 0, logText, fmt.Errorf("qemu exited early for %s: %s", instanceName, logText)
}

func isQEMUHostPortConflict(logText string) bool {
	lower := strings.ToLower(logText)
	return strings.Contains(lower, "hostfwd") &&
		(strings.Contains(lower, "address already in use") ||
			strings.Contains(lower, "could not set up host forwarding") ||
			strings.Contains(lower, "failed to set up host forwarding"))
}

func (m *Manager) createOverlay(manifest config.Manifest, overlayPath string) error {
	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}

	args := []string{
		"create",
		"-f", "qcow2",
		"-F", manifest.ImageFormat,
		"-b", manifest.Image,
		overlayPath,
	}
	if output, err := exec.Command(qemuImg, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("create overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

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

	grace := time.Duration(instance.StopGracePeriodSec) * time.Second
	if grace <= 0 {
		grace = config.DefaultStopGracePeriodSec * time.Second
	}

	if instance.QMPPath != "" && requestPowerdown(instance.QMPPath) {
		if waitForExit(instance.PID, grace) {
			return nil
		}
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

// requestPowerdown dials the QMP socket, completes the handshake, and
// sends system_powerdown. It returns true only when the server ACKs the
// command. Any failure (missing socket, handshake timeout, QMP error) is
// swallowed and reported as false so the caller falls through to SIGTERM.
// If the HOLOS_DEBUG_QMP environment variable is set, failures are logged
// to stderr to aid debugging.
func requestPowerdown(socketPath string) bool {
	client, err := qmp.Dial(socketPath, qmpHandshakeTimeout)
	if err != nil {
		if os.Getenv("HOLOS_DEBUG_QMP") != "" {
			fmt.Fprintf(os.Stderr, "qmp dial %s: %v\n", socketPath, err)
		}
		return false
	}
	defer client.Close()
	if err := client.Powerdown(qmpHandshakeTimeout); err != nil {
		if os.Getenv("HOLOS_DEBUG_QMP") != "" {
			fmt.Fprintf(os.Stderr, "qmp powerdown %s: %v\n", socketPath, err)
		}
		return false
	}
	return true
}

// waitForExit polls processAlive at 250ms intervals and returns true as
// soon as the process is no longer alive. Returns false on timeout.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return !processAlive(pid)
}

func (m *Manager) stopAllInstances(instances []InstanceRecord) {
	for idx := range instances {
		_ = m.stopInstance(instances[idx])
		instances[idx].Status = "stopped"
		instances[idx].PID = 0
		instances[idx].LastExitTime = time.Now().UTC()
	}
}

func (m *Manager) removeInstanceDirs(instances []InstanceRecord) {
	for _, inst := range instances {
		_ = os.RemoveAll(inst.WorkDir)
	}
}

// processAlive reports whether pid refers to a running QEMU process we
// started. The signal-0 probe alone is insufficient because Linux PIDs are
// recycled; after a long-running state file or a host reboot, `pid` may
// point at an unrelated process. We defend against that by also checking
// that /proc/<pid>/comm starts with "qemu-". Cheap on Linux (holos is
// Linux-only) and enough to avoid ever SIGTERM'ing a stranger.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		// On non-Linux hosts /proc is absent. Fall back to the bare
		// signal check so tests and dev environments still work.
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(string(comm)), "qemu-")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
