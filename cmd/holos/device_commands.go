package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zeroecco/holos/internal/vfio"
)

func runDevices(args []string) error {
	flags := newFlagSet("devices")
	gpuOnly := flags.Bool("gpu", false, "show only GPU devices")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *gpuOnly {
		groups, err := vfio.ListIOMMUGroups()
		if err != nil {
			return err
		}
		diagnostics := vfio.DiagnoseGPUs(groups)
		if len(diagnostics) == 0 {
			writeNoGPUsFound(os.Stdout)
			return nil
		}
		return writeGPUTable(os.Stdout, diagnostics)
	}

	groups, err := vfio.ListIOMMUGroups()
	if err != nil {
		return err
	}
	writeIOMMUGroups(os.Stdout, groups)
	return nil
}

func writeGPUTable(output io.Writer, gpus []vfio.GPUDiagnostic) error {
	writer := newTableWriter(output)
	fmt.Fprintln(writer, "PCI\tTYPE\tVENDOR:DEVICE\tDRIVER\tIOMMU\tDIAGNOSTICS")
	for _, gpu := range gpus {
		dev := gpu.Device
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\t%s\n",
			dev.Address, dev.ClassName, pciVendorDevice(dev), dev.Driver, dev.IOMMUGroup, formatGPUDiagnostics(gpu.Notes))
	}
	return writer.Flush()
}

func formatGPUDiagnostics(notes []string) string {
	if len(notes) == 0 {
		return tablePlaceholder
	}
	return strings.Join(notes, "; ")
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
