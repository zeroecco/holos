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

func applyDomainDisks(serviceName string, svc *compose.Service, d Domain) ([]ImportedVolume, []string) {
	var volumes []ImportedVolume
	var warnings []string
	primaryFound := false
	extraDiskIndex := 0
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
		if disk.Driver != nil && disk.Driver.Type != "" && disk.Driver.Type != "qcow2" {
			warnings = append(warnings, fmt.Sprintf(
				"extra disk %q has format %q; only qcow2 extra disks are imported as named volumes",
				path, disk.Driver.Type))
			continue
		}
		extraDiskIndex++
		volumeName := importedDiskVolumeName(serviceName, disk, extraDiskIndex)
		svc.Volumes = append(svc.Volumes, compose.ComposeVolume{
			Type:   "volume",
			Source: volumeName,
			Target: importedDiskMountTarget(disk, extraDiskIndex),
		})
		volumes = append(volumes, ImportedVolume{Name: volumeName, SourcePath: path})
	}
	if !primaryFound {
		warnings = append(warnings, "no file-backed disk found; set image: by hand before running `holos up`")
	}
	return volumes, warnings
}

func importedDiskVolumeName(serviceName string, disk Disk, index int) string {
	suffix := sanitizeName(disk.Target.Dev)
	if suffix == "" {
		suffix = fmt.Sprintf("disk-%d", index)
	}
	return sanitizeName(serviceName + "-" + suffix)
}

func importedDiskMountTarget(disk Disk, index int) string {
	name := sanitizeName(disk.Target.Dev)
	if name == "" {
		name = fmt.Sprintf("disk-%d", index)
	}
	return "/mnt/" + name
}

func applyDomainHostDevices(svc *compose.Service, d Domain) []string {
	var warnings []string
	for _, hd := range d.Devices.HostDevs {
		if hd.Type == "usb" {
			device, ok := importedUSBDevice(hd)
			if !ok {
				warnings = append(warnings, `hostdev type "usb" is missing vendor/product ids`)
				continue
			}
			svc.Devices = append(svc.Devices, device)
			warnings = append(warnings, fmt.Sprintf(
				"hostdev usb %s preserved as device metadata; USB passthrough runtime support still needs review",
				device.Source))
			continue
		}
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

func importedUSBDevice(hd HostDev) (compose.ComposeDevice, bool) {
	vendor := usbIDValue(hd.Source.Vendor)
	product := usbIDValue(hd.Source.Product)
	if vendor == "" || product == "" {
		return compose.ComposeDevice{}, false
	}
	id := "usb:" + vendor + ":" + product
	return compose.ComposeDevice{
		Source:      id,
		Target:      id,
		Permissions: "rwm",
	}, true
}

func usbIDValue(id *USBID) string {
	if id == nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id.ID)), "0x")
}

func applyDomainInterfaces(serviceName string, svc *compose.Service, d Domain) ([]ImportedNetwork, []string) {
	var networks []ImportedNetwork
	warnings := make([]string, 0, len(d.Devices.Interfaces))
	for i, iface := range d.Devices.Interfaces {
		desc := describeInterface(iface)
		source, ok := interfaceSourceValue(iface)
		if ok {
			networkName := importedInterfaceNetworkName(serviceName, iface, i)
			if svc.Networks == nil {
				svc.Networks = compose.ServiceNetworks{}
			}
			svc.Networks[networkName] = compose.ServiceNetwork{
				MacAddress: interfaceMACAddress(iface),
			}
			networks = append(networks, ImportedNetwork{
				Name:   networkName,
				Type:   iface.Type,
				Source: source,
				Model:  interfaceModel(iface),
			})
			if iface.Type == "bridge" {
				warnings = append(warnings, fmt.Sprintf(
					"interface %s imported as bridge network %q; host qemu-bridge-helper access to %q is required",
					desc, networkName, source))
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"interface %s preserved as network %q metadata; non-bridge NIC runtime support still needs review",
					desc, networkName))
			}
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"interface %s not imported. holos services share an internal subnet; expose with ports: instead",
			desc))
	}
	return networks, warnings
}

func importedInterfaceNetworkName(serviceName string, iface Interface, index int) string {
	source, ok := interfaceSourceValue(iface)
	if !ok {
		source = fmt.Sprintf("net-%d", index+1)
	}
	return sanitizeName(serviceName + "-" + source)
}

func interfaceSourceValue(iface Interface) (string, bool) {
	if iface.Source == nil {
		return "", false
	}
	switch {
	case iface.Source.Bridge != "":
		return iface.Source.Bridge, true
	case iface.Source.Dev != "":
		return iface.Source.Dev, true
	case iface.Source.Network != "":
		return iface.Source.Network, true
	default:
		return "", false
	}
}

func interfaceMACAddress(iface Interface) string {
	if iface.MAC == nil {
		return ""
	}
	return strings.TrimSpace(iface.MAC.Address)
}

func interfaceModel(iface Interface) string {
	if iface.Model == nil {
		return ""
	}
	return strings.TrimSpace(iface.Model.Type)
}
