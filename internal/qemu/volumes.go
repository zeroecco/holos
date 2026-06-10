package qemu

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	volumeIdentifierPrefix = "vol-"
	mountTagRootTarget     = "root"
	mountTagPathSeparator  = "/"
	mountTagSeparator      = "-"
	mountTagFormat         = "share%d-%s"
)

// volumeDriveID is the QEMU internal identifier used to link the -drive
// blob to the -device that exposes it; must be unique per-instance.
func volumeDriveID(name string) string {
	return volumeIdentifierPrefix + name
}

// volumeSerial is written into the virtio-blk device's serial and becomes
// the stable in-guest /dev/disk/by-id/virtio-<serial> path. Prefix keeps
// it from colliding with whatever the guest image hands out by default.
func volumeSerial(name string) string {
	return volumeIdentifierPrefix + name
}

func mountTag(index int, target string) string {
	target = mountTagTarget(target)
	return fmt.Sprintf(mountTagFormat, index, target)
}

func mountTagTarget(target string) string {
	target = strings.Trim(filepath.Clean(target), mountTagPathSeparator)
	target = strings.ReplaceAll(target, mountTagPathSeparator, mountTagSeparator)
	if target == "" || target == "." {
		target = mountTagRootTarget
	}
	return target
}
