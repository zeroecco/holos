package qemu

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func baseMachineArgs(manifest config.Manifest, spec LaunchSpec) []string {
	return []string{
		"-name", spec.Name,
		"-enable-kvm",
		"-machine", machineOptions(manifest),
		"-cpu", manifest.VM.CPUModel,
		"-smp", fmt.Sprintf("%d", manifest.VM.VCPU),
		"-m", fmt.Sprintf("%d", manifest.VM.MemoryMB),
		"-nodefaults",
		"-no-user-config",
		"-display", "none",
		"-chardev", consoleCharDevOption(spec.SerialPath, spec.LogPath),
		"-serial", "chardev:console0",
		"-chardev", qmpCharDevOption(spec.QMPPath),
		"-mon", "chardev=qmp,mode=control",
		"-device", "virtio-rng-pci",
		"-device", "virtio-balloon-pci",
	}
}

func consoleCharDevOption(serialPath, logPath string) string {
	return fmt.Sprintf("socket,id=console0,path=%s,server=on,wait=off,logfile=%s,logappend=on",
		qemuOptEscape(serialPath), qemuOptEscape(logPath))
}

func qmpCharDevOption(socketPath string) string {
	return fmt.Sprintf("socket,id=qmp,path=%s,server=on,wait=off", qemuOptEscape(socketPath))
}

func machineOptions(manifest config.Manifest) string {
	options := fmt.Sprintf("%s,accel=kvm", manifest.VM.Machine)
	if len(manifest.Devices) > 0 {
		options += ",kernel-irqchip=on"
	}
	return options
}

func firmwareArgs(manifest config.Manifest, spec LaunchSpec) []string {
	// UEFI firmware is required for GPU passthrough and optional otherwise.
	if !manifest.VM.UEFI || spec.OVMFCode == "" || spec.OVMFVars == "" {
		return nil
	}
	return []string{
		qemuArgDrive, qemuOptions(
			qemuOptIfPflash,
			qemuKeyValue(qemuOptKeyFormat, config.ImageFormatRaw),
			qemuOptReadonly,
			qemuKeyValue(qemuOptKeyFile, qemuOptEscape(spec.OVMFCode)),
		),
		qemuArgDrive, qemuOptions(
			qemuOptIfPflash,
			qemuKeyValue(qemuOptKeyFormat, config.ImageFormatRaw),
			qemuKeyValue(qemuOptKeyFile, qemuOptEscape(spec.OVMFVars)),
		),
	}
}

func rootDiskArgs(spec LaunchSpec) []string {
	return []string{
		qemuArgDrive, rootDiskOption(spec.OverlayPath),
		qemuArgDevice, "virtio-blk-pci,drive=root,bootindex=1",
	}
}

func rootDiskOption(overlayPath string) string {
	return qemuOptions(
		qemuKeyValue(qemuOptKeyID, "root"),
		qemuOptIfNone,
		qemuOptCacheWriteback,
		qemuOptDiscardUnmap,
		qemuKeyValue(qemuOptKeyFormat, config.ImageFormatQCOW2),
		qemuKeyValue(qemuOptKeyFile, qemuOptEscape(overlayPath)),
	)
}
