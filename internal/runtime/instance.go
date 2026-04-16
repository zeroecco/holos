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
)

func (m *Manager) startInstance(project string, manifest config.Manifest, index int) (InstanceRecord, error) {
	workDir := projectInstanceDir(m.stateDir, project, manifest.Name, index)
	if err := os.RemoveAll(workDir); err != nil {
		return InstanceRecord{}, fmt.Errorf("remove instance workdir: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
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

	ports, err := allocatePorts(manifest, index)
	if err != nil {
		return InstanceRecord{}, err
	}

	logPath := filepath.Join(workDir, "console.log")
	serialPath := filepath.Join(workDir, "serial.sock")
	qmpPath := filepath.Join(workDir, "qmp.sock")
	qemuLogPath := filepath.Join(workDir, "qemu.log")

	spec := qemu.LaunchSpec{
		Name:        instanceName,
		Index:       index,
		OverlayPath: overlayPath,
		SeedPath:    seedPath,
		LogPath:     logPath,
		SerialPath:  serialPath,
		QMPPath:     qmpPath,
		Ports:       ports,
	}

	if manifest.VM.UEFI {
		ovmfCode, ovmfVars, err := m.prepareUEFI(workDir)
		if err != nil {
			return InstanceRecord{}, err
		}
		spec.OVMFCode = ovmfCode
		spec.OVMFVars = ovmfVars
	}

	args, err := qemu.BuildArgs(manifest, spec)
	if err != nil {
		return InstanceRecord{}, err
	}

	qemuLog, err := os.OpenFile(qemuLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return InstanceRecord{}, fmt.Errorf("open qemu log: %w", err)
	}
	defer qemuLog.Close()

	command, err := m.qemuSystemCommand(args...)
	if err != nil {
		return InstanceRecord{}, err
	}
	command.Stdout = qemuLog
	command.Stderr = qemuLog
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := command.Start(); err != nil {
		return InstanceRecord{}, fmt.Errorf("start qemu: %w", err)
	}

	pid := command.Process.Pid
	_ = command.Process.Release()

	time.Sleep(300 * time.Millisecond)
	if !processAlive(pid) {
		content, _ := os.ReadFile(qemuLogPath)
		return InstanceRecord{}, fmt.Errorf("qemu exited early for %s: %s", instanceName, strings.TrimSpace(string(content)))
	}

	return InstanceRecord{
		Name:        instanceName,
		Index:       index,
		PID:         pid,
		Status:      "running",
		WorkDir:     workDir,
		OverlayPath: overlayPath,
		SeedPath:    seedPath,
		LogPath:     logPath,
		SerialPath:  serialPath,
		QMPPath:     qmpPath,
		Ports:       ports,
		LastStarted: time.Now().UTC(),
	}, nil
}

// restartInstance boots an existing stopped instance without recreating
// its overlay or seed image, preserving VM disk state across stop/start.
func (m *Manager) restartInstance(manifest config.Manifest, prev InstanceRecord) (InstanceRecord, error) {
	qemuLogPath := filepath.Join(prev.WorkDir, "qemu.log")

	ports, err := allocatePorts(manifest, prev.Index)
	if err != nil {
		return InstanceRecord{}, err
	}

	spec := qemu.LaunchSpec{
		Name:        prev.Name,
		Index:       prev.Index,
		OverlayPath: prev.OverlayPath,
		SeedPath:    prev.SeedPath,
		LogPath:     prev.LogPath,
		SerialPath:  prev.SerialPath,
		QMPPath:     prev.QMPPath,
		Ports:       ports,
	}

	if manifest.VM.UEFI {
		ovmfCode, err := findOVMF("HOLOS_OVMF_CODE", ovmfCodePaths)
		if err != nil {
			return InstanceRecord{}, err
		}
		spec.OVMFCode = ovmfCode
		spec.OVMFVars = filepath.Join(prev.WorkDir, "OVMF_VARS.fd")
	}

	args, err := qemu.BuildArgs(manifest, spec)
	if err != nil {
		return InstanceRecord{}, err
	}

	qemuLog, err := os.OpenFile(qemuLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return InstanceRecord{}, fmt.Errorf("open qemu log: %w", err)
	}
	defer qemuLog.Close()

	command, err := m.qemuSystemCommand(args...)
	if err != nil {
		return InstanceRecord{}, err
	}
	command.Stdout = qemuLog
	command.Stderr = qemuLog
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := command.Start(); err != nil {
		return InstanceRecord{}, fmt.Errorf("start qemu: %w", err)
	}

	pid := command.Process.Pid
	_ = command.Process.Release()

	time.Sleep(300 * time.Millisecond)
	if !processAlive(pid) {
		content, _ := os.ReadFile(qemuLogPath)
		return InstanceRecord{}, fmt.Errorf("qemu exited early for %s: %s", prev.Name, strings.TrimSpace(string(content)))
	}

	return InstanceRecord{
		Name:        prev.Name,
		Index:       prev.Index,
		PID:         pid,
		Status:      "running",
		WorkDir:     prev.WorkDir,
		OverlayPath: prev.OverlayPath,
		SeedPath:    prev.SeedPath,
		LogPath:     prev.LogPath,
		SerialPath:  prev.SerialPath,
		QMPPath:     prev.QMPPath,
		Ports:       ports,
		LastStarted: time.Now().UTC(),
	}, nil
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

func (m *Manager) stopInstance(instance InstanceRecord) error {
	if instance.PID == 0 || !processAlive(instance.PID) {
		return nil
	}

	process, err := os.FindProcess(instance.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", instance.PID, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal pid %d: %w", instance.PID, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(instance.PID) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill pid %d: %w", instance.PID, err)
	}
	return nil
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
// that /proc/<pid>/comm starts with "qemu-" — cheap on Linux (holos is
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
