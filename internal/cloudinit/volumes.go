package cloudinit

import (
	"fmt"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	volumeFilesystem        = "ext4"
	volumeMkfsCommand       = "mkfs.ext4"
	volumeFstabDefaultOpts  = "defaults,nofail"
	volumeFstabReadOnlyOpts = "ro,nofail"
	volumeMountErrorPrefix  = "holos: failed to mount volume "
	volumeCommandSeparator  = " && "
	volumeDevicePathPrefix  = "/dev/disk/by-id/virtio-"
	volumeLabelPrefix       = "vol-"
)

// volumeMountRunCmd produces a runcmd snippet per named volume that
// runs on every boot but is idempotent: mkfs only when the block device
// has no detectable filesystem, fstab edit only on first hit, and
// an explicit target mount at the end so mount failures stay visible.
//
// We use /dev/disk/by-id/virtio-<serial> because the PCI device number
// (and thus /dev/vdX naming) changes with any hardware layout tweak;
// the by-id path is stable across reboots and virtual-hardware edits.
func volumeMountRunCmd(manifest config.Manifest) []string {
	var cmds []string
	for _, m := range manifest.Mounts {
		if m.Kind != config.MountKindVolume {
			continue
		}
		cmds = append(cmds, volumeMountRunCommand(m))
	}
	return cmds
}

func volumeMountRunCommand(mount config.Mount) string {
	return strings.Join(volumeMountSteps(mount), volumeCommandSeparator)
}

func volumeMountSteps(mount config.Mount) []string {
	dev := volumeDevicePathPrefix + volumeLabel(mount.VolumeName)
	target := mount.Target
	steps := []string{volumeSettleCommand(dev)}

	// Read-only volumes can't be formatted by the guest because QEMU opens
	// the drive readonly=on. Skipping mkfs also preserves the compose :ro
	// contract end-to-end.
	if !mount.ReadOnly {
		steps = append(steps, volumeMkfsGuardCommand(dev, volumeLabel(mount.VolumeName)))
	}
	fstabOpts := volumeFstabDefaultOpts
	if mount.ReadOnly {
		fstabOpts = volumeFstabReadOnlyOpts
	}

	return append(steps,
		volumeMkdirCommand(target),
		volumeFstabAppendCommand(dev, target, fstabOpts),
		volumeMountCommand(mount.VolumeName, target),
	)
}

func volumeSettleCommand(dev string) string {
	return fmt.Sprintf("udevadm settle --exit-if-exists=%s || true", shquote(dev))
}

func volumeMkfsGuardCommand(dev, label string) string {
	return fmt.Sprintf("if [ -b %s ] && ! blkid %s >/dev/null 2>&1; then %s -F -L %s %s; fi",
		shquote(dev), shquote(dev), volumeMkfsCommand, shquote(label), shquote(dev))
}

func volumeMkdirCommand(target string) string {
	return fmt.Sprintf("mkdir -p %s", shquote(target))
}

func volumeFstabAppendCommand(dev, target, opts string) string {
	return fmt.Sprintf("grep -qE %s /etc/fstab || echo %s >> /etc/fstab",
		shquote(" "+target+" "),
		shquote(dev+" "+target+" "+volumeFilesystem+" "+opts+" 0 2"),
	)
}

func volumeMountCommand(name, target string) string {
	return fmt.Sprintf("mountpoint -q %s || mount %s || { echo %s >&2; exit 1; }",
		shquote(target), shquote(target), shquote(volumeMountErrorPrefix+name+" at "+target))
}

// shquote wraps s in single quotes and escapes any embedded single
// quotes by ending the quoted region, inserting an escaped single
// quote, and reopening. The only reliable way to embed a quote in
// a single-quoted POSIX shell string.
func shquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func volumeLabel(name string) string {
	return volumeLabelPrefix + name
}
