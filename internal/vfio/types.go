package vfio

// PCIDevice represents a PCI device discovered from sysfs, including its
// address, class, vendor/device IDs, current driver binding, and IOMMU group.
type PCIDevice struct {
	Address    string // BDF notation, e.g. "0000:01:00.0"
	Class      string // first 4 hex digits of PCI class, e.g. "0300" (VGA)
	Vendor     string // PCI vendor ID, e.g. "10de" (NVIDIA)
	DeviceID   string // PCI device ID, e.g. "2204"
	Driver     string // kernel driver, e.g. "vfio-pci", "nvidia", "nouveau"
	IOMMUGroup int    // IOMMU group number
	ClassName  string // human-readable class name, e.g. "VGA", "Audio"
}

// IOMMUGroup is a set of PCI devices that share an IOMMU group and must be
// passed through to a VM together.
type IOMMUGroup struct {
	ID      int
	Devices []PCIDevice
}
