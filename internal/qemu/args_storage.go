package qemu

import (
	"fmt"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

func virtfsArgs(mounts []config.Mount) []string {
	var args []string
	for i, mount := range mounts {
		if mount.Kind == config.MountKindVolume {
			// Named volumes are attached as virtio-blk devices, not 9p shares.
			continue
		}
		args = append(args, "-virtfs", virtfsOption(i, mount))
	}
	return args
}

func virtfsOption(index int, mount config.Mount) string {
	options := []string{
		"local",
		fmt.Sprintf("path=%s", qemuOptEscape(mount.Source)),
		// mount_tag is deterministic (see mountTag), but escaping keeps this
		// helper robust if mountTag ever allows commas.
		fmt.Sprintf("mount_tag=%s", qemuOptEscape(mountTag(index, mount.Target))),
		"security_model=none",
	}
	if mount.ReadOnly {
		options = append(options, "readonly=on")
	}
	return strings.Join(options, ",")
}

func volumeDiskArgs(volumes []VolumeAttachment) []string {
	var args []string
	for _, vol := range volumes {
		// Split form (-drive if=none + -device virtio-blk-pci) is required so
		// serial= surfaces as /dev/disk/by-id/virtio-<serial> in the guest.
		driveID := volumeDriveID(vol.Name)
		options := []string{
			qemuKeyValue(qemuOptKeyID, driveID),
			qemuOptIfNone,
			qemuKeyValue(qemuOptKeyFormat, config.ImageFormatQCOW2),
			qemuKeyValue(qemuOptKeyFile, qemuOptEscape(vol.DiskPath)),
			qemuOptCacheWriteback,
			qemuOptDiscardUnmap,
		}
		if vol.ReadOnly {
			options = append(options, qemuOptReadonly)
		}
		deviceOption := qemuOptions(
			"virtio-blk-pci",
			qemuKeyValue("drive", driveID),
			qemuKeyValue("serial", volumeSerial(vol.Name)),
		)
		args = append(args,
			qemuArgDrive, qemuOptions(options...),
			qemuArgDevice, deviceOption,
		)
	}
	return args
}
