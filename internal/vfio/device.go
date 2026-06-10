package vfio

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	pciDevicesRoot    = "/sys/bus/pci/devices"
	pciClassFile      = "class"
	pciVendorFile     = "vendor"
	pciDeviceIDFile   = "device"
	pciDriverLink     = "driver"
	pciClassVGA       = "0300"
	pciClass3D        = "0302"
	pciClassAudio     = "0403"
	pciClassEthernet  = "0200"
	pciClassNVMe      = "0108"
	pciClassSATA      = "0106"
	pciClassPCIBridge = "0604"
	pciClassHost      = "0600"
	pciClassUSB       = "0c03"
	sysfsHexPrefix    = "0x"
)

var pciClassNames = []struct {
	prefix string
	name   string
}{
	{prefix: pciClassVGA, name: "VGA"},
	{prefix: pciClass3D, name: "3D Controller"},
	{prefix: pciClassAudio, name: "Audio"},
	{prefix: pciClassEthernet, name: "Ethernet"},
	{prefix: pciClassNVMe, name: "NVMe"},
	{prefix: pciClassSATA, name: "SATA"},
	{prefix: pciClassPCIBridge, name: "PCI Bridge"},
	{prefix: pciClassHost, name: "Host Bridge"},
	{prefix: pciClassUSB, name: "USB"},
}

func readPCIDevice(address string, groupID int) PCIDevice {
	sysPath := filepath.Join(pciDevicesRoot, address)

	dev := PCIDevice{
		Address:    address,
		IOMMUGroup: groupID,
	}

	dev.Class = readSysfsHex(filepath.Join(sysPath, pciClassFile))
	dev.Class = normalizePCIClass(dev.Class)
	dev.Vendor = readSysfsHex(filepath.Join(sysPath, pciVendorFile))
	dev.DeviceID = readSysfsHex(filepath.Join(sysPath, pciDeviceIDFile))
	dev.Driver = readDriverName(filepath.Join(sysPath, pciDriverLink))
	dev.ClassName = classToName(dev.Class)

	return dev
}

func readSysfsHex(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	return strings.TrimPrefix(s, sysfsHexPrefix)
}

func normalizePCIClass(class string) string {
	if len(class) >= 4 {
		return class[:4]
	}
	return class
}

func readDriverName(driverLink string) string {
	target, err := os.Readlink(driverLink)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func classToName(class string) string {
	for _, entry := range pciClassNames {
		if strings.HasPrefix(class, entry.prefix) {
			return entry.name
		}
	}
	return class
}
