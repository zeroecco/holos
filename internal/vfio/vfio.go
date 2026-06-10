package vfio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	iommuGroupsRoot    = "/sys/kernel/iommu_groups"
	iommuDevicesSubdir = "devices"
)

var gpuClassPrefixes = []string{pciClassVGA, pciClass3D}

// ListIOMMUGroups discovers all IOMMU groups and their devices from sysfs.
func ListIOMMUGroups() ([]IOMMUGroup, error) {
	groupsPath := iommuGroupsRoot
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

		devicesPath := filepath.Join(groupsPath, entry.Name(), iommuDevicesSubdir)
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
			if isGPUClass(dev.Class) {
				gpus = append(gpus, dev)
			}
		}
	}
	return gpus, nil
}

func isGPUClass(class string) bool {
	for _, prefix := range gpuClassPrefixes {
		if strings.HasPrefix(class, prefix) {
			return true
		}
	}
	return false
}
