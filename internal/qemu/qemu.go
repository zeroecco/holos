package qemu

import (
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	qemuOptionSeparator = ","
	qemuEscapedComma    = ",,"
)

// qemuOptEscape escapes a value for inclusion inside a QEMU option
// string (the comma-delimited key=value blobs used with -chardev,
// -drive, -device, -virtfs, -netdev, etc.). QEMU's option parser
// uses `,` as the separator and treats `,,` as a literal comma, so
// any path (or other value) containing a comma must double its
// commas before interpolation. Without this, a user legitimately
// naming a directory `foo,bar` silently splits into two pseudo-
// options and the launch either fails with a cryptic "unknown
// parameter" error or, worse, quietly accepts an attacker-supplied
// suffix like `,readonly=off` appended to a bind-mount path.
func qemuOptEscape(s string) string {
	return strings.ReplaceAll(s, qemuOptionSeparator, qemuEscapedComma)
}

// BuildArgs produces the full qemu-system-x86_64 argument list for launching
// a single VM instance.
func BuildArgs(manifest config.Manifest, spec LaunchSpec) ([]string, error) {
	args := baseMachineArgs(manifest, spec)
	args = append(args, firmwareArgs(manifest, spec)...)
	args = append(args, rootDiskArgs(spec)...)

	// User-mode NIC for host connectivity and port forwarding.
	netdev, err := buildNetdev(spec.Ports, spec.SSHPort)
	if err != nil {
		return nil, err
	}
	args = append(args, networkArgs(manifest, spec, netdev)...)

	if spec.SeedPath != "" {
		args = append(args, qemuArgDrive, seedDriveOption(spec.SeedPath))
	}
	args = append(args, virtfsArgs(manifest.Mounts)...)
	args = append(args, volumeDiskArgs(spec.Volumes)...)
	args = append(args, vfioDeviceArgs(manifest.Devices)...)

	args = append(args, manifest.VM.ExtraArgs...)

	return args, nil
}

func seedDriveOption(seedPath string) string {
	return qemuOptions(
		qemuOptIfVirtio,
		qemuOptMediaCDROM,
		qemuOptReadonly,
		qemuKeyValue(qemuOptKeyFormat, config.ImageFormatRaw),
		qemuKeyValue(qemuOptKeyFile, qemuOptEscape(seedPath)),
	)
}
