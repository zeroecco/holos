package virtimport

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
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

func assertServiceDevicePCIs(t *testing.T, svc compose.Service, want []string) {
	t.Helper()

	if len(svc.Devices) != len(want) {
		t.Fatalf("devices len = %d, want %d: %+v", len(svc.Devices), len(want), svc.Devices)
	}
	for i, wantPCI := range want {
		if svc.Devices[i].PCI != wantPCI {
			t.Fatalf("devices[%d].PCI = %q, want %q: %+v", i, svc.Devices[i].PCI, wantPCI, svc.Devices)
		}
	}
}

func TestConvertFullDomain(t *testing.T) {
	t.Parallel()

	name, svc, volumes, networks, warns, err := ConvertWithResources([]byte(fullDomainXML))
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
	if len(svc.Volumes) != 1 {
		t.Fatalf("service volumes = %+v, want imported extra disk", svc.Volumes)
	}
	if got := svc.Volumes[0]; got.Type != "volume" || got.Source != "my-web-server-vdb" || got.Target != "/mnt/vdb" {
		t.Fatalf("service volume = %+v, want imported vdb volume", got)
	}
	if len(volumes) != 1 || volumes[0].Name != "my-web-server-vdb" || volumes[0].SourcePath != "/var/lib/libvirt/images/my-web-server-data.qcow2" {
		t.Fatalf("imported volumes = %+v, want vdb source", volumes)
	}
	if len(networks) != 1 || networks[0].Name != "my-web-server-default" || networks[0].Type != "network" || networks[0].Source != "default" || networks[0].Model != "virtio" {
		t.Fatalf("imported networks = %+v, want default virtio network", networks)
	}
	if got := svc.Networks["my-web-server-default"].MacAddress; got != "52:54:00:00:00:01" {
		t.Fatalf("service network mac = %q, want imported mac", got)
	}
	assertServiceDevicePCIs(t, svc, []string{"0000:01:00.0"})

	wantWarnings := []string{
		"renamed domain",       // sanitised name
		"hostdev type \"usb\"", // unsupported passthrough
		"interface",            // bridged/network NIC preserved as metadata
	}
	assertWarningsContain(t, warns, wantWarnings...)
}

func TestConvertPreservesBridgeInterfaceIntent(t *testing.T) {
	t.Parallel()

	xml := []byte(`
<domain type='kvm'>
  <name>bridgevm</name>
  <devices>
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/bridgevm.qcow2'/>
    </disk>
    <interface type='bridge'>
      <mac address='52:54:00:aa:bb:cc'/>
      <source bridge='br0'/>
      <model type='virtio'/>
    </interface>
  </devices>
</domain>
`)

	_, svc, _, networks, warns, err := ConvertWithResources(xml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(networks) != 1 || networks[0].Name != "bridgevm-br0" || networks[0].Type != "bridge" || networks[0].Source != "br0" {
		t.Fatalf("imported networks = %+v, want br0 bridge", networks)
	}
	if got := svc.Networks["bridgevm-br0"].MacAddress; got != "52:54:00:aa:bb:cc" {
		t.Fatalf("network mac = %q, want libvirt mac", got)
	}
	assertWarningsContain(t, warns, `preserved as network "bridgevm-br0" metadata`)
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
	assertWarningsContain(t, warns, "no file-backed disk")
}

func TestConvertParseError(t *testing.T) {
	t.Parallel()

	if _, _, _, err := Convert([]byte("not xml at all")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "web", want: "web"},
		{name: "mixed case and spaces", input: "My Web Server", want: "my-web-server"},
		{name: "trimmed dotted name", input: "  spaced.name  ", want: "spaced-name"},
		{name: "punctuation collapsed", input: "weird___name!!!", want: "weird-name"},
		{name: "trim separators", input: "---trim---", want: "trim"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitizeName(tc.input); got != tc.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatPCI(t *testing.T) {
	t.Parallel()

	got := formatPCI(PCIAddress{Domain: "0x0000", Bus: "0x42", Slot: "0x1f", Function: "0x3"})
	if got != "0000:42:1f.3" {
		t.Errorf("formatPCI = %q, want 0000:42:1f.3", got)
	}
}

func TestCPUModelName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cpu  *CPUConfig
		want string
	}{
		{
			name: "nil",
			want: "",
		},
		{
			name: "host passthrough",
			cpu:  &CPUConfig{Mode: "host-passthrough"},
			want: "host",
		},
		{
			name: "host model",
			cpu:  &CPUConfig{Mode: "host-model"},
			want: "host",
		},
		{
			name: "named model",
			cpu:  &CPUConfig{Mode: "custom", Model: &CPUModel{Value: " Skylake-Client-IBRS "}},
			want: "Skylake-Client-IBRS",
		},
		{
			name: "blank model",
			cpu:  &CPUConfig{Mode: "custom", Model: &CPUModel{Value: "   "}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cpuModelName(tt.cpu); got != tt.want {
				t.Fatalf("cpuModelName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDomainDiskImagePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		disk        Disk
		wantPath    string
		wantWarning string
		wantOK      bool
	}{
		{
			name:     "file disk",
			disk:     Disk{Type: "file", Device: "disk", Source: DiskSource{File: " /var/lib/vm.qcow2 "}},
			wantPath: "/var/lib/vm.qcow2",
			wantOK:   true,
		},
		{
			name:     "default file disk",
			disk:     Disk{Source: DiskSource{File: "/var/lib/default.raw"}},
			wantPath: "/var/lib/default.raw",
			wantOK:   true,
		},
		{
			name: "cdrom skipped",
			disk: Disk{Type: "file", Device: "cdrom", Source: DiskSource{File: "/srv/seed.iso"}},
		},
		{
			name: "floppy skipped",
			disk: Disk{Type: "file", Device: "floppy", Source: DiskSource{File: "/srv/floppy.img"}},
		},
		{
			name:        "non file disk warns",
			disk:        Disk{Type: "block", Device: "disk", Target: DiskTarget{Dev: "vda"}},
			wantWarning: `disk "vda" has type "block" (only file-backed disks are imported)`,
		},
		{
			name:        "network disk warns",
			disk:        Disk{Type: "network", Device: "disk", Target: DiskTarget{Dev: "vdb"}},
			wantWarning: `disk "vdb" has type "network" (only file-backed disks are imported)`,
		},
		{
			name: "blank source skipped",
			disk: Disk{Type: "file", Device: "disk", Source: DiskSource{File: "   "}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path, warning, ok := domainDiskImagePath(tt.disk)
			if path != tt.wantPath || warning != tt.wantWarning || ok != tt.wantOK {
				t.Fatalf("domainDiskImagePath() = (%q, %q, %v), want (%q, %q, %v)",
					path, warning, ok, tt.wantPath, tt.wantWarning, tt.wantOK)
			}
		})
	}
}

func TestApplyDomainVMConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		domain     Domain
		wantMemory int
		wantUEFI   bool
	}{
		{
			name: "current memory preferred",
			domain: Domain{
				Memory:        Memory{Unit: "MiB", Value: "1024"},
				CurrentMemory: Memory{Unit: "MiB", Value: "512"},
			},
			wantMemory: 512,
		},
		{
			name: "falls back to memory",
			domain: Domain{
				Memory:        Memory{Unit: "MiB", Value: "1024"},
				CurrentMemory: Memory{Unit: "MiB", Value: "0"},
			},
			wantMemory: 1024,
		},
		{
			name: "invalid memory ignored",
			domain: Domain{
				Memory:        Memory{Unit: "pages", Value: "1024"},
				CurrentMemory: Memory{Unit: "MiB", Value: "-1"},
			},
		},
		{
			name: "loader enables uefi",
			domain: Domain{
				OS: OSConfig{Loader: &Loader{Path: " /usr/share/OVMF/OVMF_CODE.fd "}},
			},
			wantUEFI: true,
		},
		{
			name: "blank loader ignored",
			domain: Domain{
				OS: OSConfig{Loader: &Loader{Path: "  "}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var svc compose.Service
			applyDomainVMConfig(&svc, tt.domain)

			if svc.VM.MemoryMB != tt.wantMemory {
				t.Fatalf("memory_mb = %d, want %d", svc.VM.MemoryMB, tt.wantMemory)
			}
			if svc.VM.UEFI != tt.wantUEFI {
				t.Fatalf("uefi = %v, want %v", svc.VM.UEFI, tt.wantUEFI)
			}
		})
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

func TestMemoryUnitMultiplier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		unit string
		want int64
		ok   bool
	}{
		{unit: "", want: 1 << 10, ok: true},
		{unit: " KiB ", want: 1 << 10, ok: true},
		{unit: "bytes", want: 1, ok: true},
		{unit: "MB", want: 1 << 20, ok: true},
		{unit: "g", want: 1 << 30, ok: true},
		{unit: "TiB", want: 1 << 40, ok: true},
		{unit: "pages", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			t.Parallel()

			got, ok := memoryUnitMultiplier(tt.unit)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("memoryUnitMultiplier(%q) = (%d, %v), want (%d, %v)", tt.unit, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseVirshDomainNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		out  string
		want []string
	}{
		{name: "trims blanks", out: "\n web \n\n db\n\tbatch\t\n", want: []string{"web", "db", "batch"}},
		{name: "trims crlf", out: "web\r\n db\r\n", want: []string{"web", "db"}},
		{name: "empty output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseVirshDomainNames([]byte(tt.out))
			assertStringSliceEqual(t, "parseVirshDomainNames", got, tt.want)
		})
	}
}

func TestExitErrorStderr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStderr string
		wantOK     bool
	}{
		{
			name:       "exit error with stderr",
			err:        &exec.ExitError{Stderr: []byte("  failed to connect\n")},
			wantStderr: "failed to connect",
			wantOK:     true,
		},
		{name: "exit error without stderr", err: &exec.ExitError{}},
		{name: "other error", err: fmt.Errorf("lookup failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStderr, gotOK := exitErrorStderr(tt.err)
			if gotStderr != tt.wantStderr || gotOK != tt.wantOK {
				t.Fatalf("exitErrorStderr(%T) = (%q, %v), want (%q, %v)",
					tt.err, gotStderr, gotOK, tt.wantStderr, tt.wantOK)
			}
		})
	}
}

func TestExitErrorHasStderr(t *testing.T) {
	t.Parallel()

	if exitErrorHasStderr(&exec.ExitError{}) {
		t.Fatal("exitErrorHasStderr(empty) = true, want false")
	}
	if !exitErrorHasStderr(&exec.ExitError{Stderr: []byte("failed\n")}) {
		t.Fatal("exitErrorHasStderr(stderr) = false, want true")
	}
}

func assertWarningsContain(t *testing.T, warns []string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !warningsContain(warns, want) {
			t.Errorf("expected warning containing %q, got %v", want, warns)
		}
	}
}

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func warningsContain(warns []string, substr string) bool {
	for _, warning := range warns {
		if strings.Contains(warning, substr) {
			return true
		}
	}
	return false
}
