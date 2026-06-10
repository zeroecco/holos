package virtimport

import (
	"fmt"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

func applyDomainVMConfig(svc *compose.Service, d Domain) {
	// Memory: prefer <currentMemory> (the live ceiling) and fall back
	// to <memory> (the boot-time max) so a guest with ballooning
	// configured doesn't end up with the wrong value.
	memBytes, memErr := memoryToBytes(d.CurrentMemory)
	if memErr != nil || memBytes <= 0 {
		memBytes, memErr = memoryToBytes(d.Memory)
	}
	if memErr == nil && memBytes > 0 {
		svc.VM.MemoryMB = int(memBytes / (1 << 20))
	}

	if d.VCPU.Value > 0 {
		svc.VM.VCPU = d.VCPU.Value
	}

	if m := d.OS.Type.Machine; m != "" {
		svc.VM.Machine = simplifyMachine(m)
	}

	if cpuModel := cpuModelName(d.CPU); cpuModel != "" {
		svc.VM.CPUModel = cpuModel
	}

	if d.OS.Loader != nil && strings.TrimSpace(d.OS.Loader.Path) != "" {
		svc.VM.UEFI = true
	}
}

func applyDomainDisks(svc *compose.Service, d Domain) []string {
	var warnings []string
	primaryFound := false
	for _, disk := range d.Devices.Disks {
		path, warning, ok := domainDiskImagePath(disk)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if !ok {
			continue
		}
		if !primaryFound {
			svc.Image = path
			if disk.Driver != nil && disk.Driver.Type != "" {
				svc.ImageFormat = disk.Driver.Type
			}
			primaryFound = true
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"extra disk %q skipped; declare it under top-level volumes: and reference it from the service",
			path))
	}
	if !primaryFound {
		warnings = append(warnings, "no file-backed disk found; set image: by hand before running `holos up`")
	}
	return warnings
}

func applyDomainHostDevices(svc *compose.Service, d Domain) []string {
	var warnings []string
	for _, hd := range d.Devices.HostDevs {
		if hd.Type != "pci" {
			warnings = append(warnings, fmt.Sprintf("hostdev type %q is not supported (only pci passthrough imports)", hd.Type))
			continue
		}
		if hd.Source.Address == nil {
			continue
		}
		svc.Devices = append(svc.Devices, compose.ComposeDevice{
			PCI: formatPCI(*hd.Source.Address),
		})
	}
	return warnings
}

func interfaceWarnings(d Domain) []string {
	warnings := make([]string, 0, len(d.Devices.Interfaces))
	for _, iface := range d.Devices.Interfaces {
		desc := describeInterface(iface)
		warnings = append(warnings, fmt.Sprintf(
			"interface %s not imported. holos services share an internal subnet; expose with ports: instead",
			desc))
	}
	return warnings
}
