package qemu

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
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
	return strings.ReplaceAll(s, ",", ",,")
}

// PortMapping is a resolved host-to-guest TCP port forward assigned to a
// running instance.
type PortMapping struct {
	Name      string `json:"name"`
	HostPort  int    `json:"host_port"`
	GuestPort int    `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

// LaunchSpec carries the per-instance paths and port mappings needed to
// construct QEMU arguments. The runtime populates this after creating the
// overlay, seed image, and allocating ports.
type LaunchSpec struct {
	Name        string
	Index       int
	OverlayPath string
	SeedPath    string
	LogPath     string
	SerialPath  string
	QMPPath     string
	Ports       []PortMapping
	OVMFCode    string // path to OVMF_CODE.fd (read-only, shared)
	OVMFVars    string // path to per-instance OVMF_VARS.fd copy (writable)
	// SSHPort is the host-side TCP port that should forward to the
	// guest's sshd (22/tcp). The runtime allocates it on first boot to
	// back `holos exec`. Zero means no ssh forward was requested; the
	// user-mode netdev is built without the sshd hostfwd so we don't
	// occupy ports unnecessarily when the feature is disabled.
	SSHPort int
	// Volumes are resolved named-volume attachments for this instance.
	// Each entry becomes a virtio-blk block device exposed to the guest
	// with a stable serial so udev creates /dev/disk/by-id/virtio-<serial>.
	Volumes []VolumeAttachment
}

// VolumeAttachment is a single qcow2-backed block device attached to an
// instance, produced by the runtime when materialising named volumes.
type VolumeAttachment struct {
	// Name is the logical volume name from the compose file (e.g. "data").
	Name string
	// DiskPath is the host-visible path the guest should open. The
	// runtime points this at a workdir symlink that targets the
	// project-level qcow2 file, so tearing the workdir down never
	// removes the volume data.
	DiskPath string
	// ReadOnly maps the compose `:ro` suffix on a named volume to
	// QEMU's drive readonly=on. Without this the runtime silently
	// dropped the flag; operators who asked for a read-only volume
	// still got a writable drive and could corrupt shared data.
	ReadOnly bool
}

// BuildArgs produces the full qemu-system-x86_64 argument list for launching
// a single VM instance.
func BuildArgs(manifest config.Manifest, spec LaunchSpec) ([]string, error) {
	machineOpts := fmt.Sprintf("%s,accel=kvm", manifest.VM.Machine)
	if len(manifest.Devices) > 0 {
		machineOpts += ",kernel-irqchip=on"
	}

	args := []string{
		"-name", spec.Name,
		"-enable-kvm",
		"-machine", machineOpts,
		"-cpu", manifest.VM.CPUModel,
		"-smp", fmt.Sprintf("%d", manifest.VM.VCPU),
		"-m", fmt.Sprintf("%d", manifest.VM.MemoryMB),
		"-nodefaults",
		"-no-user-config",
		"-display", "none",
		"-chardev", fmt.Sprintf("socket,id=console0,path=%s,server=on,wait=off,logfile=%s,logappend=on",
			qemuOptEscape(spec.SerialPath), qemuOptEscape(spec.LogPath)),
		"-serial", "chardev:console0",
		"-chardev", fmt.Sprintf("socket,id=qmp,path=%s,server=on,wait=off", qemuOptEscape(spec.QMPPath)),
		"-mon", "chardev=qmp,mode=control",
		"-device", "virtio-rng-pci",
		"-device", "virtio-balloon-pci",
	}

	// UEFI firmware (required for GPU passthrough, optional otherwise).
	if manifest.VM.UEFI && spec.OVMFCode != "" && spec.OVMFVars != "" {
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", qemuOptEscape(spec.OVMFCode)),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", qemuOptEscape(spec.OVMFVars)),
		)
	}

	args = append(args,
		"-drive", fmt.Sprintf("if=virtio,cache=writeback,discard=unmap,format=qcow2,file=%s", qemuOptEscape(spec.OverlayPath)),
	)

	// User-mode NIC for host connectivity and port forwarding.
	netdev, err := buildNetdev(spec.Ports, spec.SSHPort)
	if err != nil {
		return nil, err
	}
	userDevice := "virtio-net-pci,netdev=net0"
	if manifest.InternalNetwork != nil {
		if userMAC := manifest.InternalNetwork.UserMAC(spec.Index); userMAC != "" {
			userDevice += ",mac=" + userMAC
		}
	}
	args = append(args, "-netdev", netdev, "-device", userDevice)

	// Socket multicast NIC for inter-VM networking.
	if manifest.InternalNetwork != nil {
		mac := manifest.InternalNetwork.InstanceMAC(spec.Index)
		socketNetdev := fmt.Sprintf("socket,id=net1,mcast=%s:%d",
			manifest.InternalNetwork.MulticastGroup,
			manifest.InternalNetwork.MulticastPort)
		args = append(args,
			"-netdev", socketNetdev,
			"-device", fmt.Sprintf("virtio-net-pci,netdev=net1,mac=%s", mac))
	}

	if spec.SeedPath != "" {
		args = append(args, "-drive", fmt.Sprintf("if=virtio,media=cdrom,readonly=on,format=raw,file=%s", qemuOptEscape(spec.SeedPath)))
	}

	for i, mount := range manifest.Mounts {
		if mount.Kind == config.MountKindVolume {
			// Named volumes are attached below as virtio-blk devices,
			// not 9p shares. A mkfs'ed block device behaves much more
			// like a normal disk (filesystem features, xattrs, fsync
			// semantics) than virtfs does.
			continue
		}
		options := []string{
			"local",
			fmt.Sprintf("path=%s", qemuOptEscape(mount.Source)),
			// mount_tag is a small, deterministic token we build
			// ourselves (see mountTag), so escaping is defensive
			// rather than necessary: it costs nothing and
			// insulates us from a future mountTag that allows
			// commas.
			fmt.Sprintf("mount_tag=%s", qemuOptEscape(mountTag(i, mount.Target))),
			"security_model=none",
		}
		if mount.ReadOnly {
			options = append(options, "readonly=on")
		}
		args = append(args, "-virtfs", strings.Join(options, ","))
	}

	for _, vol := range spec.Volumes {
		// Split form (-drive if=none + -device virtio-blk-pci) is
		// required so we can set serial=, which surfaces as
		// /dev/disk/by-id/virtio-<serial> via udev inside the guest.
		// The `if=virtio` shorthand doesn't accept serial.
		driveID := volumeDriveID(vol.Name)
		driveOpts := fmt.Sprintf("id=%s,if=none,format=qcow2,file=%s,cache=writeback,discard=unmap",
			driveID, qemuOptEscape(vol.DiskPath))
		if vol.ReadOnly {
			// QEMU honors readonly=on on the -drive node; the
			// virtio-blk device inherits the mode and the guest
			// sees the disk as read-only without any in-guest
			// configuration.
			driveOpts += ",readonly=on"
		}
		args = append(args,
			"-drive", driveOpts,
			"-device", fmt.Sprintf("virtio-blk-pci,drive=%s,serial=%s", driveID, volumeSerial(vol.Name)),
		)
	}

	// VFIO PCI device passthrough (GPUs, NICs, etc.).
	for _, dev := range manifest.Devices {
		if dev.PCI == "" {
			continue
		}
		vfioOpts := fmt.Sprintf("vfio-pci,host=%s", qemuOptEscape(dev.PCI))
		if dev.ROMFile != "" {
			vfioOpts += fmt.Sprintf(",romfile=%s", qemuOptEscape(dev.ROMFile))
		}
		args = append(args, "-device", vfioOpts)
	}

	args = append(args, manifest.VM.ExtraArgs...)

	return args, nil
}

func buildNetdev(ports []PortMapping, sshPort int) (string, error) {
	options := []string{"user,id=net0"}
	for _, port := range ports {
		if port.Protocol != "tcp" {
			return "", fmt.Errorf("unsupported port mapping protocol %q", port.Protocol)
		}
		options = append(options, fmt.Sprintf("hostfwd=tcp:127.0.0.1:%d-:%d", port.HostPort, port.GuestPort))
	}
	if sshPort > 0 {
		options = append(options, fmt.Sprintf("hostfwd=tcp:127.0.0.1:%d-:22", sshPort))
	}
	return strings.Join(options, ","), nil
}

// volumeDriveID is the QEMU internal identifier used to link the -drive
// blob to the -device that exposes it; must be unique per-instance.
func volumeDriveID(name string) string {
	return "vol-" + name
}

// volumeSerial is written into the virtio-blk device's serial and becomes
// the stable in-guest /dev/disk/by-id/virtio-<serial> path. Prefix keeps
// it from colliding with whatever the guest image hands out by default.
func volumeSerial(name string) string {
	return "vol-" + name
}

func mountTag(index int, target string) string {
	target = strings.Trim(filepath.Clean(target), "/")
	target = strings.ReplaceAll(target, "/", "-")
	if target == "" || target == "." {
		target = "root"
	}
	return fmt.Sprintf("share%d-%s", index, target)
}
