package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

const (
	vgaDeviceArg  = "-device"
	vgaDeviceName = "VGA"
)

func resolveServiceVMConfig(svc Service, resolver composeImageResolver, devices []config.Device) (config.VMConfig, error) {
	vcpu := svc.VM.VCPU
	if vcpu == 0 {
		cpus, err := composeCPUs(svc)
		if err != nil {
			return config.VMConfig{}, err
		}
		vcpu = composeVCPU(cpus)
	}

	memMB := svc.VM.MemoryMB
	if memMB == 0 {
		resolved, err := resolveVMMemoryMB(svc, resolver)
		if err != nil {
			return config.VMConfig{}, err
		}
		memMB = resolved
	}

	diskSizeBytes, err := resolveVMDiskSizeBytes(svc.VM.DiskSize)
	if err != nil {
		return config.VMConfig{}, err
	}

	machine := resolveVMMachine(svc.VM.Machine)
	cpuModel := resolveVMCPUModel(svc.VM.CPUModel)

	uefi := resolveVMUEFI(svc.VM.UEFI, devices)
	extraArgs := resolveVMExtraArgs(svc.VM.ExtraArgs, uefi, resolver.RequiresVGA(svc.Image))

	return config.VMConfig{
		VCPU:          vcpu,
		MemoryMB:      memMB,
		DiskSizeBytes: diskSizeBytes,
		Machine:       machine,
		CPUModel:      cpuModel,
		UEFI:          uefi,
		ExtraArgs:     extraArgs,
	}, nil
}

func resolveVMUEFI(explicit bool, devices []config.Device) bool {
	return explicit || len(devices) > 0
}

func resolveVMMachine(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return config.DefaultMachine
}

func resolveVMCPUModel(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return config.DefaultCPUModel
}

func resolveVMExtraArgs(extraArgs []string, uefi bool, requiresVGA bool) []string {
	resolved := append([]string(nil), extraArgs...)
	if uefi || !requiresVGA {
		return resolved
	}
	// Debian 13's BIOS GRUB path can stall at "Booting Debian GNU/Linux"
	// with holos' -nodefaults serial-only device layout. Supplying a
	// headless VGA device satisfies GRUB's gfxterm setup while keeping the
	// serial console and QEMU display disabled.
	return append([]string{vgaDeviceArg, vgaDeviceName}, resolved...)
}

func resolveVMMemoryMB(svc Service, resolver composeImageResolver) (int, error) {
	memLimit := composeMemLimit(svc)
	memMB, err := composeMemoryMB(memLimit)
	if err != nil {
		return 0, err
	}
	return applyImageMinimumMemory(memMB, memLimit, svc.Image, resolver), nil
}

func applyImageMinimumMemory(memMB int, memLimit string, image string, resolver composeImageResolver) int {
	if !isBlankScalarString(memLimit) {
		return memMB
	}
	return max(memMB, resolver.MinMemoryMB(image))
}

func resolveVMDiskSizeBytes(raw string) (int64, error) {
	if isBlankScalarString(raw) {
		return 0, nil
	}
	size, err := parseVolumeSize(raw)
	if err != nil {
		return 0, fmt.Errorf("vm.disk_size: %w", err)
	}
	return size, nil
}
