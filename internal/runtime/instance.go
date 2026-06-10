package runtime

import (
	"fmt"
	"os"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	portRetryAttempts = 5
)

func (m *Manager) startInstance(project string, manifest config.Manifest, index int) (InstanceRecord, error) {
	workDir := projectInstanceDir(m.stateDir, project, manifest.Name, index)
	paths := newInstancePaths(workDir)
	if err := os.RemoveAll(workDir); err != nil {
		return InstanceRecord{}, fmt.Errorf("remove instance workdir: %w", err)
	}
	// 0700 mirrors the rest of the state tree: this dir holds the
	// overlay qcow2, qmp socket, and console.log; nothing in there is
	// meant for other users on the host.
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return InstanceRecord{}, fmt.Errorf("create instance workdir: %w", err)
	}

	if err := m.createOverlay(manifest, paths.overlay); err != nil {
		return InstanceRecord{}, err
	}

	instanceName := manifest.InstanceName(index)

	seedPath, err := m.createSeedImage(manifest, instanceName, index, workDir)
	if err != nil {
		return InstanceRecord{}, err
	}

	volumes, err := materializeInstanceVolumes(m.stateDir, project, workDir, manifest.Mounts)
	if err != nil {
		return InstanceRecord{}, err
	}

	baseSpec := startLaunchSpec(instanceName, index, paths, seedPath, volumes)
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
	pid, err := m.launchWithPortRetry(manifest, baseSpec, paths.qemuLog, func() (qemu.LaunchSpec, error) {
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

	return startedInstanceRecord(instanceName, index, workDir, paths, seedPath, manifest, pid, ports, sshPort, time.Now().UTC()), nil
}

func startLaunchSpec(instanceName string, index int, paths instancePaths, seedPath string, volumes []qemu.VolumeAttachment) qemu.LaunchSpec {
	return qemu.LaunchSpec{
		Name:        instanceName,
		Index:       index,
		OverlayPath: paths.overlay,
		SeedPath:    seedPath,
		LogPath:     paths.consoleLog,
		SerialPath:  paths.serialSocket,
		QMPPath:     paths.qmpSocket,
		Volumes:     volumes,
	}
}

// InspectLaunchSpec reconstructs the per-instance QEMU launch inputs from a
// saved instance record and a matching resolved manifest. It does not allocate
// ports, create files, or otherwise mutate state.
func InspectLaunchSpec(manifest config.Manifest, instance InstanceRecord) qemu.LaunchSpec {
	volumes := inspectVolumeAttachments(instance.WorkDir, manifest.Mounts)
	spec := qemu.LaunchSpec{
		Name:        instance.Name,
		Index:       instance.Index,
		OverlayPath: instance.OverlayPath,
		SeedPath:    instance.SeedPath,
		LogPath:     instance.LogPath,
		SerialPath:  instance.SerialPath,
		QMPPath:     instance.QMPPath,
		Ports:       instance.Ports,
		SSHPort:     instance.SSHPort,
		Volumes:     volumes,
	}
	if manifest.VM.UEFI && instance.WorkDir != "" {
		spec.OVMFVars = newInstancePaths(instance.WorkDir).ovmfVars
	}
	return spec
}

func inspectVolumeAttachments(workDir string, mounts []config.Mount) []qemu.VolumeAttachment {
	if workDir == "" {
		return nil
	}
	var volumes []qemu.VolumeAttachment
	for _, mount := range mounts {
		if mount.Kind != config.MountKindVolume {
			continue
		}
		volumes = append(volumes, volumeAttachmentForMount(mount, volumeLinkPath(workDir, mount.VolumeName)))
	}
	return volumes
}

func startedInstanceRecord(instanceName string, index int, workDir string, paths instancePaths, seedPath string, manifest config.Manifest, pid int, ports []qemu.PortMapping, sshPort int, startedAt time.Time) InstanceRecord {
	record := InstanceRecord{
		Name:               instanceName,
		Index:              index,
		WorkDir:            workDir,
		OverlayPath:        paths.overlay,
		SeedPath:           seedPath,
		LogPath:            paths.consoleLog,
		SerialPath:         paths.serialSocket,
		QMPPath:            paths.qmpSocket,
		StopGracePeriodSec: manifest.StopGracePeriodSec,
	}
	return instanceRecordWithLaunchResult(record, pid, ports, sshPort, startedAt)
}

func instanceRecordWithLaunchResult(record InstanceRecord, pid int, ports []qemu.PortMapping, sshPort int, startedAt time.Time) InstanceRecord {
	record.PID = pid
	record.Status = InstanceStatusRunning
	record.Ports = ports
	record.SSHPort = sshPort
	record.LastStarted = startedAt
	return record
}
