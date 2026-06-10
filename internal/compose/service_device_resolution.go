package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func resolveServiceDevices(svc Service) ([]config.Device, error) {
	devices := make([]config.Device, 0, len(svc.Devices))
	for i, d := range svc.Devices {
		device, ok, err := resolveComposeDevice(d)
		if err != nil {
			return nil, fmt.Errorf("device %d pci %q: %w", i, d.PCI, err)
		}
		if !ok {
			continue
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func resolveComposeDevice(d ComposeDevice) (config.Device, bool, error) {
	if d.Raw != "" || d.PCI == "" {
		return config.Device{}, false, nil
	}
	pci := normalizePCIAddress(d.PCI)
	if err := config.ValidatePCIAddress(pci); err != nil {
		return config.Device{}, false, err
	}
	return config.Device{
		PCI:     pci,
		ROMFile: d.ROMFile,
	}, true, nil
}
