package main

import (
	"bytes"
	"testing"

	"github.com/zeroecco/holos/internal/vfio"
)

const (
	testPCINvidiaAddress = "0000:01:00.0"
	testPCINvidiaVendor  = "10de"
	testPCINvidiaDevice  = "2204"
	testPCINvidiaDriver  = "nvidia"
	testPCIAMDAddress    = "0000:02:00.0"
	testPCIAMDVendor     = "1002"
	testPCIAMDDevice     = "73bf"
	testPCIAMDDriver     = "vfio-pci"
	testPCIAudioAddress  = "0000:00:1f.3"
	testPCIAudioVendor   = "8086"
	testPCIAudioDevice   = "a0c8"
	testPCIAudioDriver   = "snd_hda_intel"
	testPCISMBusAddress  = "0000:00:1f.4"
	testPCISMBusVendor   = "8086"
	testPCISMBusDevice   = "a0a3"
	testNvidiaIOMMUGroup = 12
	testAMDIOMMUGroup    = 14
	testPCIGroupID       = 7
)

func TestWriteGPUTable(t *testing.T) {
	t.Parallel()

	gpus := []vfio.PCIDevice{
		{
			Address:    testPCINvidiaAddress,
			ClassName:  "VGA",
			Vendor:     testPCINvidiaVendor,
			DeviceID:   testPCINvidiaDevice,
			Driver:     testPCINvidiaDriver,
			IOMMUGroup: testNvidiaIOMMUGroup,
		},
		{
			Address:    testPCIAMDAddress,
			ClassName:  "3D Controller",
			Vendor:     testPCIAMDVendor,
			DeviceID:   testPCIAMDDevice,
			Driver:     testPCIAMDDriver,
			IOMMUGroup: testAMDIOMMUGroup,
		},
	}

	var out bytes.Buffer
	if err := writeGPUTable(&out, gpus); err != nil {
		t.Fatalf("writeGPUTable: %v", err)
	}
	want := "PCI           TYPE           VENDOR:DEVICE  DRIVER    IOMMU\n" +
		"0000:01:00.0  VGA            10de:2204      nvidia    12\n" +
		"0000:02:00.0  3D Controller  1002:73bf      vfio-pci  14\n"
	if got := out.String(); got != want {
		t.Fatalf("gpu table = %q, want %q", got, want)
	}
}

func TestWriteNoGPUsFound(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writeNoGPUsFound(&out)
	if got, want := out.String(), "no GPUs found\n"; got != want {
		t.Fatalf("no GPUs output = %q, want %q", got, want)
	}
}

func TestWriteIOMMUGroups(t *testing.T) {
	t.Parallel()

	groups := []vfio.IOMMUGroup{
		{
			ID: testPCIGroupID,
			Devices: []vfio.PCIDevice{
				{
					Address:   testPCIAudioAddress,
					ClassName: "Audio",
					Vendor:    testPCIAudioVendor,
					DeviceID:  testPCIAudioDevice,
					Driver:    testPCIAudioDriver,
				},
				{
					Address:   testPCISMBusAddress,
					ClassName: "SMBus",
					Vendor:    testPCISMBusVendor,
					DeviceID:  testPCISMBusDevice,
				},
			},
		},
	}

	var out bytes.Buffer
	writeIOMMUGroups(&out, groups)
	want := "IOMMU Group 7:\n" +
		"  0000:00:1f.3  Audio  8086:a0c8  [snd_hda_intel]\n" +
		"  0000:00:1f.4  SMBus  8086:a0a3  [-]\n"
	if got := out.String(); got != want {
		t.Fatalf("iommu groups = %q, want %q", got, want)
	}
}
