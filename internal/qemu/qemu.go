package qemu

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

type PortMapping struct {
	Name      string `json:"name"`
	HostPort  int    `json:"host_port"`
	GuestPort int    `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

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
}

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
		"-chardev", fmt.Sprintf("socket,id=console0,path=%s,server=on,wait=off,logfile=%s,logappend=on", spec.SerialPath, spec.LogPath),
		"-serial", "chardev:console0",
		"-chardev", fmt.Sprintf("socket,id=qmp,path=%s,server=on,wait=off", spec.QMPPath),
		"-mon", "chardev=qmp,mode=control",
		"-device", "virtio-rng-pci",
		"-device", "virtio-balloon-pci",
	}

	// UEFI firmware (required for GPU passthrough, optional otherwise).
	if manifest.VM.UEFI && spec.OVMFCode != "" && spec.OVMFVars != "" {
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", spec.OVMFCode),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", spec.OVMFVars),
		)
	}

	args = append(args,
		"-drive", fmt.Sprintf("if=virtio,cache=writeback,discard=unmap,format=qcow2,file=%s", spec.OverlayPath),
	)

	// User-mode NIC for host connectivity and port forwarding.
	netdev, err := buildNetdev(spec.Ports)
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
		args = append(args, "-drive", fmt.Sprintf("if=virtio,media=cdrom,readonly=on,format=raw,file=%s", spec.SeedPath))
	}

	for i, mount := range manifest.Mounts {
		options := []string{
			"local",
			fmt.Sprintf("path=%s", mount.Source),
			fmt.Sprintf("mount_tag=%s", mountTag(i, mount.Target)),
			"security_model=none",
		}
		if mount.ReadOnly {
			options = append(options, "readonly=on")
		}
		args = append(args, "-virtfs", strings.Join(options, ","))
	}

	// VFIO PCI device passthrough (GPUs, NICs, etc.).
	for _, dev := range manifest.Devices {
		if dev.PCI == "" {
			continue
		}
		vfioOpts := fmt.Sprintf("vfio-pci,host=%s", dev.PCI)
		if dev.ROMFile != "" {
			vfioOpts += fmt.Sprintf(",romfile=%s", dev.ROMFile)
		}
		args = append(args, "-device", vfioOpts)
	}

	args = append(args, manifest.VM.ExtraArgs...)

	return args, nil
}

func buildNetdev(ports []PortMapping) (string, error) {
	options := []string{"user,id=net0"}
	for _, port := range ports {
		if port.Protocol != "tcp" {
			return "", fmt.Errorf("unsupported port mapping protocol %q", port.Protocol)
		}
		options = append(options, fmt.Sprintf("hostfwd=tcp:127.0.0.1:%d-:%d", port.HostPort, port.GuestPort))
	}
	return strings.Join(options, ","), nil
}

func mountTag(index int, target string) string {
	target = strings.Trim(filepath.Clean(target), "/")
	target = strings.ReplaceAll(target, "/", "-")
	if target == "" || target == "." {
		target = "root"
	}
	return fmt.Sprintf("share%d-%s", index, target)
}
