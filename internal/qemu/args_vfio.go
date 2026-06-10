package qemu

import "github.com/zeroecco/holos/internal/config"

const (
	vfioPCIDevice  = "vfio-pci"
	vfioHostKey    = "host"
	vfioROMFileKey = "romfile"
)

func vfioDeviceArgs(devices []config.Device) []string {
	var args []string
	for _, dev := range devices {
		if dev.PCI == "" {
			continue
		}
		args = append(args, qemuArgDevice, vfioDeviceOption(dev))
	}
	return args
}

func vfioDeviceOption(dev config.Device) string {
	options := []string{
		vfioPCIDevice,
		qemuKeyValue(vfioHostKey, qemuOptEscape(dev.PCI)),
	}
	if dev.ROMFile != "" {
		options = append(options, qemuKeyValue(vfioROMFileKey, qemuOptEscape(dev.ROMFile)))
	}
	return qemuOptions(options...)
}
