package vfio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSysfsHex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "class")
	if err := os.WriteFile(path, []byte("0x030000\n"), 0o600); err != nil {
		t.Fatalf("write sysfs hex: %v", err)
	}
	if got := readSysfsHex(path); got != "030000" {
		t.Fatalf("readSysfsHex = %q, want 030000", got)
	}
	if got := readSysfsHex(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("readSysfsHex missing = %q, want empty", got)
	}
}

func TestNormalizePCIClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		class string
		want  string
	}{
		{name: "full class", class: "030000", want: "0300"},
		{name: "already normalized", class: "0300", want: "0300"},
		{name: "short class", class: "03", want: "03"},
		{name: "empty", class: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizePCIClass(tt.class); got != tt.want {
				t.Fatalf("normalizePCIClass(%q) = %q, want %q", tt.class, got, tt.want)
			}
		})
	}
}

func TestClassToName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		class string
		want  string
	}{
		{name: "vga", class: pciClassVGA + "00", want: "VGA"},
		{name: "3d controller", class: pciClass3D + "00", want: "3D Controller"},
		{name: "audio", class: pciClassAudio + "00", want: "Audio"},
		{name: "ethernet", class: pciClassEthernet + "00", want: "Ethernet"},
		{name: "nvme", class: pciClassNVMe + "00", want: "NVMe"},
		{name: "sata", class: pciClassSATA + "00", want: "SATA"},
		{name: "pci bridge", class: pciClassPCIBridge + "00", want: "PCI Bridge"},
		{name: "host bridge", class: pciClassHost + "00", want: "Host Bridge"},
		{name: "usb", class: pciClassUSB + "00", want: "USB"},
		{name: "unknown", class: "ffff", want: "ffff"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := classToName(tt.class); got != tt.want {
				t.Fatalf("classToName(%q) = %q, want %q", tt.class, got, tt.want)
			}
		})
	}
}

func TestIsGPUClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		class string
		want  bool
	}{
		{name: "vga", class: pciClassVGA, want: true},
		{name: "vga subclass", class: pciClassVGA + "00", want: true},
		{name: "3d", class: pciClass3D, want: true},
		{name: "3d subclass", class: pciClass3D + "00", want: true},
		{name: "audio", class: pciClassAudio},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isGPUClass(tt.class); got != tt.want {
				t.Fatalf("isGPUClass(%q) = %v, want %v", tt.class, got, tt.want)
			}
		})
	}
}
