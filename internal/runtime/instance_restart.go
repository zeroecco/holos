package runtime

import (
	"fmt"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

// restartInstance boots an existing stopped instance without recreating
// its overlay or seed image, preserving VM disk state across stop/start.
func (m *Manager) restartInstance(project string, manifest config.Manifest, prev InstanceRecord) (InstanceRecord, error) {
	paths := newInstancePaths(prev.WorkDir)

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
	baseSpec := restartLaunchSpec(prev, volumes)

	if manifest.VM.UEFI {
		firmware, err := ResolveOVMFFirmware()
		if err != nil {
			return InstanceRecord{}, err
		}
		baseSpec.OVMFCode = firmware.CodePath
		baseSpec.OVMFVars = paths.ovmfVars
	}

	var ports []qemu.PortMapping
	var sshPort int
	first := true
	pid, err := m.launchWithPortRetry(manifest, baseSpec, paths.qemuLog, func() (qemu.LaunchSpec, error) {
		var err error
		ports, err = allocatePorts(manifest, prev.Index)
		if err != nil {
			return qemu.LaunchSpec{}, err
		}
		sshPort = prev.SSHPort
		if shouldAllocateRestartSSHPort(first, sshPort) {
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

	return restartedInstanceRecord(prev, manifest, pid, ports, sshPort, time.Now().UTC()), nil
}

func shouldAllocateRestartSSHPort(firstAttempt bool, previousPort int) bool {
	if !firstAttempt || previousPort == 0 {
		return true
	}
	return ensureTCPPortAvailable(defaultHostAddr, previousPort) != nil
}

func restartLaunchSpec(prev InstanceRecord, volumes []qemu.VolumeAttachment) qemu.LaunchSpec {
	return qemu.LaunchSpec{
		Name:        prev.Name,
		Index:       prev.Index,
		OverlayPath: prev.OverlayPath,
		SeedPath:    prev.SeedPath,
		LogPath:     prev.LogPath,
		SerialPath:  prev.SerialPath,
		QMPPath:     prev.QMPPath,
		Volumes:     volumes,
	}
}

func restartedInstanceRecord(prev InstanceRecord, manifest config.Manifest, pid int, ports []qemu.PortMapping, sshPort int, startedAt time.Time) InstanceRecord {
	record := InstanceRecord{
		Name:               prev.Name,
		Index:              prev.Index,
		WorkDir:            prev.WorkDir,
		OverlayPath:        prev.OverlayPath,
		SeedPath:           prev.SeedPath,
		LogPath:            prev.LogPath,
		SerialPath:         prev.SerialPath,
		QMPPath:            prev.QMPPath,
		StopGracePeriodSec: manifest.StopGracePeriodSec,
	}
	return instanceRecordWithLaunchResult(record, pid, ports, sshPort, startedAt)
}
