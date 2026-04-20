package virtimport

import (
	"strings"
	"testing"
)

const fullDomainXML = `
<domain type='kvm'>
  <name>My Web Server</name>
  <uuid>11111111-2222-3333-4444-555555555555</uuid>
  <memory unit='KiB'>2097152</memory>
  <currentMemory unit='KiB'>2097152</currentMemory>
  <vcpu placement='static'>4</vcpu>
  <os>
    <type arch='x86_64' machine='pc-q35-7.2'>hvm</type>
    <loader readonly='yes' type='pflash'>/usr/share/OVMF/OVMF_CODE.fd</loader>
  </os>
  <cpu mode='host-passthrough' check='none'/>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/my-web-server.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='/var/lib/libvirt/images/my-web-server-data.qcow2'/>
      <target dev='vdb' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <source file='/srv/iso/seed.iso'/>
      <target dev='sda' bus='sata'/>
    </disk>
    <interface type='network'>
      <mac address='52:54:00:00:00:01'/>
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <hostdev mode='subsystem' type='pci' managed='yes'>
      <source>
        <address domain='0x0000' bus='0x01' slot='0x00' function='0x0'/>
      </source>
    </hostdev>
    <hostdev mode='subsystem' type='usb'>
      <source><vendor id='0x1d6b'/><product id='0x0002'/></source>
    </hostdev>
  </devices>
</domain>
`

func TestConvertFullDomain(t *testing.T) {
	t.Parallel()

	name, svc, warns, err := Convert([]byte(fullDomainXML))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if name != "my-web-server" {
		t.Errorf("name = %q, want my-web-server", name)
	}
	if svc.VM.VCPU != 4 {
		t.Errorf("vcpu = %d, want 4", svc.VM.VCPU)
	}
	if svc.VM.MemoryMB != 2048 {
		t.Errorf("memory_mb = %d, want 2048", svc.VM.MemoryMB)
	}
	if svc.VM.Machine != "q35" {
		t.Errorf("machine = %q, want q35", svc.VM.Machine)
	}
	if svc.VM.CPUModel != "host" {
		t.Errorf("cpu_model = %q, want host", svc.VM.CPUModel)
	}
	if !svc.VM.UEFI {
		t.Error("expected UEFI=true (loader present)")
	}
	if svc.Image != "/var/lib/libvirt/images/my-web-server.qcow2" {
		t.Errorf("image = %q, want /var/lib/libvirt/images/my-web-server.qcow2", svc.Image)
	}
	if svc.ImageFormat != "qcow2" {
		t.Errorf("image_format = %q, want qcow2", svc.ImageFormat)
	}
	if len(svc.Devices) != 1 || svc.Devices[0].PCI != "0000:01:00.0" {
		t.Errorf("devices = %+v, want one entry 0000:01:00.0", svc.Devices)
	}

	wantWarnings := []string{
		"renamed domain",       // sanitised name
		"extra disk",           // second qcow2
		"hostdev type \"usb\"", // unsupported passthrough
		"interface",            // bridged/network NIC dropped
	}
	for _, want := range wantWarnings {
		if !containsAny(warns, want) {
			t.Errorf("expected a warning containing %q, got %v", want, warns)
		}
	}
}

const minimalDomainXML = `
<domain type='kvm'>
  <name>tiny</name>
  <memory unit='MiB'>256</memory>
  <vcpu>1</vcpu>
  <os>
    <type machine='pc-i440fx-6.2'>hvm</type>
  </os>
  <devices>
    <disk type='file' device='disk'>
      <source file='/tmp/tiny.raw'/>
      <driver type='raw'/>
    </disk>
  </devices>
</domain>
`

func TestConvertMinimalDomain(t *testing.T) {
	t.Parallel()

	name, svc, warns, err := Convert([]byte(minimalDomainXML))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if name != "tiny" {
		t.Errorf("name = %q, want tiny", name)
	}
	if svc.VM.MemoryMB != 256 {
		t.Errorf("memory_mb = %d, want 256", svc.VM.MemoryMB)
	}
	if svc.VM.Machine != "pc" {
		t.Errorf("machine = %q, want pc", svc.VM.Machine)
	}
	if svc.VM.UEFI {
		t.Error("UEFI should be false when no loader is present")
	}
	if svc.ImageFormat != "raw" {
		t.Errorf("image_format = %q, want raw", svc.ImageFormat)
	}
	if len(warns) != 0 {
		t.Errorf("expected no warnings, got %v", warns)
	}
}

func TestConvertNoDisk(t *testing.T) {
	t.Parallel()

	xml := `<domain type='kvm'><name>nodisk</name><memory unit='MiB'>128</memory><vcpu>1</vcpu><os><type>hvm</type></os><devices/></domain>`
	_, svc, warns, err := Convert([]byte(xml))
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if svc.Image != "" {
		t.Errorf("image should be empty, got %q", svc.Image)
	}
	if !containsAny(warns, "no file-backed disk") {
		t.Errorf("expected warning about missing disk, got %v", warns)
	}
}

func TestConvertParseError(t *testing.T) {
	t.Parallel()

	if _, _, _, err := Convert([]byte("not xml at all")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"web":             "web",
		"My Web Server":   "my-web-server",
		"  spaced.name  ": "spaced-name",
		"weird___name!!!": "weird-name",
		"---trim---":      "trim",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPCI(t *testing.T) {
	t.Parallel()

	got := formatPCI(PCIAddress{Domain: "0x0000", Bus: "0x42", Slot: "0x1f", Function: "0x3"})
	if got != "0000:42:1f.3" {
		t.Errorf("formatPCI = %q, want 0000:42:1f.3", got)
	}
}

func TestMemoryToBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mem  Memory
		want int64
	}{
		{Memory{Unit: "KiB", Value: "1024"}, 1024 * 1024},
		{Memory{Unit: "", Value: "2048"}, 2048 * 1024},
		{Memory{Unit: "MiB", Value: "256"}, 256 * 1024 * 1024},
		{Memory{Unit: "GiB", Value: "1"}, 1 << 30},
		{Memory{Unit: "bytes", Value: "4096"}, 4096},
	}
	for _, c := range cases {
		got, err := memoryToBytes(c.mem)
		if err != nil {
			t.Errorf("memoryToBytes(%+v) error: %v", c.mem, err)
			continue
		}
		if got != c.want {
			t.Errorf("memoryToBytes(%+v) = %d, want %d", c.mem, got, c.want)
		}
	}
}

func containsAny(warns []string, substr string) bool {
	for _, w := range warns {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}
