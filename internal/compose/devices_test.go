package compose

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func assertDevice(t *testing.T, got config.Device, pci, romFile string) {
	t.Helper()

	if got.PCI != pci || got.ROMFile != romFile {
		t.Fatalf("device = %+v, want PCI %q ROM %q", got, pci, romFile)
	}
}

func TestUEFIAutoEnabledWithDevices(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)
	writeTestImage(t, dir)

	cases := []struct {
		name     string
		uefi     bool
		devices  []ComposeDevice
		wantUEFI bool
		why      string
	}{
		{"no-devices-no-uefi", false, nil, false, "no PCI devices, no explicit flag -> SeaBIOS"},
		{"explicit-uefi", true, nil, true, "operator asked for UEFI, no devices"},
		{"devices-force-uefi", false, []ComposeDevice{{PCI: "0000:01:00.0"}}, true, "PCI passthrough requires OVMF"},
		{"devices-and-explicit", true, []ComposeDevice{{PCI: "0000:01:00.0"}}, true, "both set, idempotent"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := &File{
				Name: "uefitest",
				Services: map[string]Service{
					"vm": {
						Image:   "./base.qcow2",
						VM:      VM{UEFI: c.uefi},
						Devices: c.devices,
					},
				},
			}
			project, err := file.Resolve(dir, stateDir)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			got := project.Services["vm"].VM.UEFI
			if got != c.wantUEFI {
				t.Errorf("%s: UEFI = %v, want %v", c.why, got, c.wantUEFI)
			}
		})
	}
}

func TestResolveRejectsInvalidPCIAddress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := testStateDir(dir)
	writeTestImage(t, dir)

	file := &File{
		Name: "badpci",
		Services: map[string]Service{
			"vm": {
				Image:   "./base.qcow2",
				Devices: []ComposeDevice{{PCI: "01:00.8"}},
			},
		},
	}
	_, err := file.Resolve(dir, stateDir)
	assertErrorContains(t, err, "pci")
}

func TestResolveComposeDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		device  ComposeDevice
		wantOK  bool
		wantPCI string
		wantROM string
		wantErr string
	}{
		{name: "pci device", device: ComposeDevice{PCI: "01:00.0", ROMFile: "gpu.rom"}, wantOK: true, wantPCI: "0000:01:00.0", wantROM: "gpu.rom"},
		{name: "raw compatibility device ignored", device: ComposeDevice{Raw: "/dev/kvm"}, wantOK: false},
		{name: "non pci mapping ignored", device: ComposeDevice{Source: "/dev/kvm", Target: "/dev/kvm"}, wantOK: false},
		{name: "invalid pci", device: ComposeDevice{PCI: "01:00.8"}, wantErr: "must match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := resolveComposeDevice(tt.device)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("resolveComposeDevice: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			assertDevice(t, got, tt.wantPCI, tt.wantROM)
		})
	}
}

func TestNormalizePCIAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "short bus address", raw: "01:00.0", want: "0000:01:00.0"},
		{name: "full address", raw: "0000:01:00.0", want: "0000:01:00.0"},
		{name: "unexpected shape unchanged", raw: "01.0", want: "01.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizePCIAddress(tt.raw); got != tt.want {
				t.Fatalf("normalizePCIAddress = %q, want %q", got, tt.want)
			}
		})
	}
}
