package vfio

import (
	"fmt"
	"sort"
	"strings"
)

const (
	vfioPCIDriver = "vfio-pci"
	nvidiaVendor  = "10de"
)

// GPUDiagnostic is a GPU row enriched with passthrough readiness notes.
type GPUDiagnostic struct {
	Device PCIDevice
	Notes  []string
}

// DiagnoseGPUs returns VGA/3D devices with actionable passthrough notes based
// on current driver binding and IOMMU group membership.
func DiagnoseGPUs(groups []IOMMUGroup) []GPUDiagnostic {
	var diagnostics []GPUDiagnostic
	for _, group := range groups {
		for _, dev := range group.Devices {
			if !isGPUClass(dev.Class) {
				continue
			}
			diagnostics = append(diagnostics, GPUDiagnostic{
				Device: dev,
				Notes:  gpuDiagnosticNotes(dev, group.Devices),
			})
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		return diagnostics[i].Device.Address < diagnostics[j].Device.Address
	})
	return diagnostics
}

func gpuDiagnosticNotes(gpu PCIDevice, group []PCIDevice) []string {
	var notes []string
	if gpu.Driver != vfioPCIDriver {
		notes = append(notes, vfioDriverNote(gpu))
	}
	if peers := iommuGroupPeerNotes(gpu, group); peers != "" {
		notes = append(notes, peers)
	}
	if strings.HasPrefix(gpu.Class, pciClassVGA) && !hasSameSlotAudio(gpu, group) {
		notes = append(notes, "no same-slot audio function found; check whether the GPU has a paired audio device")
	}
	if strings.EqualFold(gpu.Vendor, nvidiaVendor) {
		notes = append(notes, "NVIDIA GPU: keep UEFI enabled and add rom_file if the guest fails to initialize the card")
	}
	if len(notes) == 0 {
		notes = append(notes, "ready")
	}
	return notes
}

func vfioDriverNote(gpu PCIDevice) string {
	driver := gpu.Driver
	if driver == "" {
		driver = "unbound"
	}
	return fmt.Sprintf("driver=%s; bind with: sudo modprobe vfio-pci && echo %s %s | sudo tee /sys/bus/pci/drivers/vfio-pci/new_id",
		driver, gpu.Vendor, gpu.DeviceID)
}

func iommuGroupPeerNotes(gpu PCIDevice, group []PCIDevice) string {
	var peers []string
	for _, dev := range group {
		if dev.Address == gpu.Address {
			continue
		}
		peers = append(peers, fmt.Sprintf("%s %s %s:%s [%s]",
			dev.Address, dev.ClassName, dev.Vendor, dev.DeviceID, driverOrPlaceholder(dev.Driver)))
	}
	if len(peers) == 0 {
		return ""
	}
	sort.Strings(peers)
	return "IOMMU group also contains " + strings.Join(peers, ", ") + "; pass the whole group or improve isolation"
}

func hasSameSlotAudio(gpu PCIDevice, group []PCIDevice) bool {
	prefix, ok := pciSlotPrefix(gpu.Address)
	if !ok {
		return true
	}
	for _, dev := range group {
		if dev.Address == gpu.Address {
			continue
		}
		if !strings.HasPrefix(dev.Address, prefix) {
			continue
		}
		if strings.HasPrefix(dev.Class, pciClassAudio) {
			return true
		}
	}
	return false
}

func pciSlotPrefix(address string) (string, bool) {
	idx := strings.LastIndex(address, ".")
	if idx <= 0 {
		return "", false
	}
	return address[:idx+1], true
}

func driverOrPlaceholder(driver string) string {
	if driver == "" {
		return "-"
	}
	return driver
}
