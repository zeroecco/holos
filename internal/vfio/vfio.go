package vfio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PCIDevice represents a PCI device discovered from sysfs.
type PCIDevice struct {
	Address    string // e.g. "0000:01:00.0"
	Class      string // e.g. "0300" (VGA)
	Vendor     string // e.g. "10de"
	DeviceID   string // e.g. "2204"
	Driver     string // e.g. "vfio-pci", "nvidia", "nouveau"
	IOMMUGroup int
	ClassName  string // human readable
}

// IOMMUGroup represents a group of PCI devices.
type IOMMUGroup struct {
	ID      int
	Devices []PCIDevice
}

// ListIOMMUGroups discovers all IOMMU groups and their devices from sysfs.
func ListIOMMUGroups() ([]IOMMUGroup, error) {
	groupsPath := "/sys/kernel/iommu_groups"
	entries, err := os.ReadDir(groupsPath)
	if err != nil {
		return nil, fmt.Errorf("read iommu groups (is IOMMU enabled?): %w", err)
	}

	var groups []IOMMUGroup
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		var groupID int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &groupID); err != nil {
			continue
		}

		devicesPath := filepath.Join(groupsPath, entry.Name(), "devices")
		deviceEntries, err := os.ReadDir(devicesPath)
		if err != nil {
			continue
		}

		var devices []PCIDevice
		for _, devEntry := range deviceEntries {
			dev := readPCIDevice(devEntry.Name(), groupID)
			devices = append(devices, dev)
		}

		if len(devices) > 0 {
			groups = append(groups, IOMMUGroup{ID: groupID, Devices: devices})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	return groups, nil
}

// ListGPUs returns only VGA/3D controller devices (class 0x0300 or 0x0302).
func ListGPUs() ([]PCIDevice, error) {
	groups, err := ListIOMMUGroups()
	if err != nil {
		return nil, err
	}

	var gpus []PCIDevice
	for _, group := range groups {
		for _, dev := range group.Devices {
			if strings.HasPrefix(dev.Class, "0300") || strings.HasPrefix(dev.Class, "0302") {
				gpus = append(gpus, dev)
			}
		}
	}
	return gpus, nil
}

func readPCIDevice(address string, groupID int) PCIDevice {
	sysPath := filepath.Join("/sys/bus/pci/devices", address)

	dev := PCIDevice{
		Address:    address,
		IOMMUGroup: groupID,
	}

	dev.Class = readSysfsHex(filepath.Join(sysPath, "class"))
	if len(dev.Class) >= 4 {
		dev.Class = dev.Class[:4]
	}
	dev.Vendor = readSysfsHex(filepath.Join(sysPath, "vendor"))
	dev.DeviceID = readSysfsHex(filepath.Join(sysPath, "device"))
	dev.Driver = readDriverName(filepath.Join(sysPath, "driver"))
	dev.ClassName = classToName(dev.Class)

	return dev
}

func readSysfsHex(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	return strings.TrimPrefix(s, "0x")
}

func readDriverName(driverLink string) string {
	target, err := os.Readlink(driverLink)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func classToName(class string) string {
	switch {
	case strings.HasPrefix(class, "0300"):
		return "VGA"
	case strings.HasPrefix(class, "0302"):
		return "3D Controller"
	case strings.HasPrefix(class, "0403"):
		return "Audio"
	case strings.HasPrefix(class, "0200"):
		return "Ethernet"
	case strings.HasPrefix(class, "0108"):
		return "NVMe"
	case strings.HasPrefix(class, "0106"):
		return "SATA"
	case strings.HasPrefix(class, "0604"):
		return "PCI Bridge"
	case strings.HasPrefix(class, "0600"):
		return "Host Bridge"
	case strings.HasPrefix(class, "0c03"):
		return "USB"
	default:
		return class
	}
}
