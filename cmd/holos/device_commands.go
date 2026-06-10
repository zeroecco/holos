package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/vfio"
)

func runDevices(args []string) error {
	flags := newFlagSet("devices")
	gpuOnly := flags.Bool("gpu", false, "show only GPU devices")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *gpuOnly {
		gpus, err := vfio.ListGPUs()
		if err != nil {
			return err
		}
		if len(gpus) == 0 {
			writeNoGPUsFound(os.Stdout)
			return nil
		}
		return writeGPUTable(os.Stdout, gpus)
	}

	groups, err := vfio.ListIOMMUGroups()
	if err != nil {
		return err
	}
	writeIOMMUGroups(os.Stdout, groups)
	return nil
}

func writeGPUTable(output io.Writer, gpus []vfio.PCIDevice) error {
	writer := newTableWriter(output)
	fmt.Fprintln(writer, "PCI\tTYPE\tVENDOR:DEVICE\tDRIVER\tIOMMU")
	for _, gpu := range gpus {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\n",
			gpu.Address, gpu.ClassName, pciVendorDevice(gpu), gpu.Driver, gpu.IOMMUGroup)
	}
	return writer.Flush()
}

func writeNoGPUsFound(output io.Writer) {
	fmt.Fprintln(output, "no GPUs found")
}

func writeIOMMUGroups(output io.Writer, groups []vfio.IOMMUGroup) {
	for _, group := range groups {
		fmt.Fprintf(output, "IOMMU Group %d:\n", group.ID)
		for _, dev := range group.Devices {
			fmt.Fprintf(output, "  %s  %s  %s  [%s]\n",
				dev.Address, dev.ClassName, pciVendorDevice(dev), tableValue(dev.Driver))
		}
	}
}

func pciVendorDevice(dev vfio.PCIDevice) string {
	return dev.Vendor + ":" + dev.DeviceID
}
