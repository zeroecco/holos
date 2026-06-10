package vfio

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiagnoseGPUsReportsActionableNotes(t *testing.T) {
	t.Parallel()

	groups := []IOMMUGroup{
		{
			ID: 12,
			Devices: []PCIDevice{
				{
					Address:    "0000:01:00.0",
					Class:      pciClassVGA,
					ClassName:  "VGA",
					Vendor:     "10de",
					DeviceID:   "2204",
					Driver:     "nvidia",
					IOMMUGroup: 12,
				},
				{
					Address:   "0000:01:00.1",
					Class:     pciClassAudio,
					ClassName: "Audio",
					Vendor:    "10de",
					DeviceID:  "1aef",
					Driver:    "snd_hda_intel",
				},
				{
					Address:   "0000:00:14.0",
					Class:     "0c03",
					ClassName: "USB",
					Vendor:    "8086",
					DeviceID:  "43ed",
					Driver:    "xhci_hcd",
				},
			},
		},
	}

	diagnostics := DiagnoseGPUs(groups)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1: %#v", len(diagnostics), diagnostics)
	}
	notes := strings.Join(diagnostics[0].Notes, "\n")
	assertContains(t, notes, "driver=nvidia; bind with: sudo modprobe vfio-pci && echo 10de 2204 | sudo tee /sys/bus/pci/drivers/vfio-pci/new_id")
	assertContains(t, notes, "IOMMU group also contains 0000:00:14.0 USB 8086:43ed [xhci_hcd], 0000:01:00.1 Audio 10de:1aef [snd_hda_intel]; pass the whole group or improve isolation")
	assertContains(t, notes, "NVIDIA GPU: keep UEFI enabled and add rom_file if the guest fails to initialize the card")
	if strings.Contains(notes, "no same-slot audio") {
		t.Fatalf("diagnostics unexpectedly reported missing audio: %s", notes)
	}
}

func TestDiagnoseGPUsReportsMissingVGAAudioAndUnboundDriver(t *testing.T) {
	t.Parallel()

	diagnostics := DiagnoseGPUs([]IOMMUGroup{
		{
			ID: 4,
			Devices: []PCIDevice{
				{
					Address:    "0000:03:00.0",
					Class:      pciClassVGA,
					ClassName:  "VGA",
					Vendor:     "1002",
					DeviceID:   "73bf",
					IOMMUGroup: 4,
				},
			},
		},
	})
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1: %#v", len(diagnostics), diagnostics)
	}
	notes := strings.Join(diagnostics[0].Notes, "\n")
	assertContains(t, notes, "driver=unbound; bind with: sudo modprobe vfio-pci && echo 1002 73bf | sudo tee /sys/bus/pci/drivers/vfio-pci/new_id")
	assertContains(t, notes, "no same-slot audio function found")
}

func TestDiagnoseGPUsReadyForBound3DController(t *testing.T) {
	t.Parallel()

	diagnostics := DiagnoseGPUs([]IOMMUGroup{
		{
			ID: 14,
			Devices: []PCIDevice{
				{
					Address:    "0000:02:00.0",
					Class:      pciClass3D,
					ClassName:  "3D Controller",
					Vendor:     "1002",
					DeviceID:   "73bf",
					Driver:     vfioPCIDriver,
					IOMMUGroup: 14,
				},
			},
		},
	})

	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics length = %d, want 1: %#v", len(diagnostics), diagnostics)
	}
	if want := []string{"ready"}; !reflect.DeepEqual(diagnostics[0].Notes, want) {
		t.Fatalf("notes = %#v, want %#v", diagnostics[0].Notes, want)
	}
}

func TestDiagnoseGPUsSortsByPCIAddress(t *testing.T) {
	t.Parallel()

	diagnostics := DiagnoseGPUs([]IOMMUGroup{
		{ID: 2, Devices: []PCIDevice{{Address: "0000:02:00.0", Class: pciClass3D, Vendor: "1002", DeviceID: "73bf", Driver: vfioPCIDriver}}},
		{ID: 1, Devices: []PCIDevice{{Address: "0000:01:00.0", Class: pciClass3D, Vendor: "1002", DeviceID: "73bf", Driver: vfioPCIDriver}}},
	})

	got := []string{diagnostics[0].Device.Address, diagnostics[1].Device.Address}
	want := []string{"0000:01:00.0", "0000:02:00.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("addresses = %#v, want %#v", got, want)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q in:\n%s", needle, haystack)
	}
}
