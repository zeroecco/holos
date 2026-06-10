package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
)

func TestWriteImportOutputWritesStdout(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "default", output: ""},
		{name: "dash", output: importStdoutOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := compose.File{
				Name: "imported",
				Services: map[string]compose.Service{
					"vm": {Image: "debian:13"},
				},
			}

			originalStdout := os.Stdout
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe stdout: %v", err)
			}
			os.Stdout = writer
			defer func() {
				os.Stdout = originalStdout
			}()

			err = writeImportOutput(tt.output, file, nil)
			if closeErr := writer.Close(); closeErr != nil {
				t.Fatalf("close stdout pipe: %v", closeErr)
			}
			if err != nil {
				t.Fatalf("writeImportOutput: %v", err)
			}

			var out bytes.Buffer
			if _, err := io.Copy(&out, reader); err != nil {
				t.Fatalf("read stdout: %v", err)
			}
			assertImportedComposeYAML(t, out.String())
		})
	}
}

func TestWriteImportOutputWritesComposeFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "compose.yaml")
	file := compose.File{
		Name: "imported",
		Services: map[string]compose.Service{
			"vm": {Image: "debian:13"},
		},
	}

	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = writer
	defer func() {
		os.Stderr = originalStderr
	}()

	if err := writeImportOutput(output, file, nil); err != nil {
		t.Fatalf("writeImportOutput: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	var stderr bytes.Buffer
	if _, err := io.Copy(&stderr, reader); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	wantSummary := "wrote " + output + " (1 service(s))\n"
	if got := stderr.String(); got != wantSummary {
		t.Fatalf("stderr = %q, want %q", got, wantSummary)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	assertImportedComposeYAML(t, string(data))

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if got := info.Mode().Perm(); got != importOutputPerm {
		t.Fatalf("output mode = %v, want %v", got, importOutputPerm)
	}
}

func TestImportAccumulatorComposeFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		explicit string
		order    []string
		want     string
	}{
		{name: "explicit", explicit: "custom", order: []string{"first"}, want: "custom"},
		{name: "first imported", order: []string{"first", "second"}, want: "first"},
		{name: "fallback", want: "imported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			acc := &importAccumulator{
				file:  compose.File{Services: map[string]compose.Service{}},
				order: tt.order,
			}
			if got := acc.composeFile(tt.explicit).Name; got != tt.want {
				t.Fatalf("composeFile(%q).Name = %q, want %q", tt.explicit, got, tt.want)
			}
		})
	}
}

func TestImportAccumulatorPrefixesWarningsWithServiceName(t *testing.T) {
	t.Parallel()

	acc := newImportAccumulator()
	xml := []byte(`
<domain type='kvm'>
  <name>My Web Server</name>
  <devices>
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/my-web-server.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
  </devices>
</domain>
`)

	if err := acc.addDomain("domain.xml", xml); err != nil {
		t.Fatalf("addDomain: %v", err)
	}
	want := `my-web-server: renamed domain "My Web Server" to "my-web-server" to satisfy compose naming rules`
	assertStringSliceEqual(t, "warnings", acc.warnings, []string{want})
}

func TestImportAccumulatorDeclaresImportedExtraDiskVolumes(t *testing.T) {
	t.Parallel()

	acc := newImportAccumulator()
	xml := []byte(`
<domain type='kvm'>
  <name>web</name>
  <devices>
    <disk type='file' device='disk'>
      <driver type='qcow2'/>
      <source file='/var/lib/libvirt/images/web.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='disk'>
      <driver type='qcow2'/>
      <source file='/var/lib/libvirt/images/web-data.qcow2'/>
      <target dev='vdb' bus='virtio'/>
    </disk>
  </devices>
</domain>
`)

	if err := acc.addDomain("web.xml", xml); err != nil {
		t.Fatalf("addDomain: %v", err)
	}
	file := acc.composeFile("")
	svc := file.Services["web"]
	if len(svc.Volumes) != 1 || svc.Volumes[0].Source != "web-vdb" || svc.Volumes[0].Target != "/mnt/vdb" {
		t.Fatalf("service volumes = %+v, want imported vdb mount", svc.Volumes)
	}
	volume, ok := file.Volumes["web-vdb"]
	if !ok {
		t.Fatalf("top-level volumes = %+v, missing web-vdb", file.Volumes)
	}
	if got := volume.DriverOpts["source"]; got != "/var/lib/libvirt/images/web-data.qcow2" {
		t.Fatalf("volume source = %q, want imported disk path", got)
	}
}

func TestImportAccumulatorDeclaresImportedBridgeNetworks(t *testing.T) {
	t.Parallel()

	acc := newImportAccumulator()
	xml := []byte(`
<domain type='kvm'>
  <name>web</name>
  <devices>
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/web.qcow2'/>
    </disk>
    <interface type='bridge'>
      <mac address='52:54:00:00:00:01'/>
      <source bridge='br0'/>
      <model type='virtio'/>
    </interface>
  </devices>
</domain>
`)

	if err := acc.addDomain("web.xml", xml); err != nil {
		t.Fatalf("addDomain: %v", err)
	}
	file := acc.composeFile("")
	svc := file.Services["web"]
	if got := svc.Networks["web-br0"].MacAddress; got != "52:54:00:00:00:01" {
		t.Fatalf("service network mac = %q, want imported mac", got)
	}
	network, ok := file.Networks["web-br0"]
	if !ok {
		t.Fatalf("top-level networks = %+v, missing web-br0", file.Networks)
	}
	if got := network.DriverOpts["holos.import.source"]; got != "br0" {
		t.Fatalf("network source = %q, want br0", got)
	}
	wantWarning := `web: interface (type=bridge, bridge=br0) imported as bridge network "web-br0"; host qemu-bridge-helper access to "br0" is required`
	if len(acc.warnings) != 1 || !strings.Contains(acc.warnings[0], wantWarning) {
		t.Fatalf("warnings = %+v, want %q", acc.warnings, wantWarning)
	}
}

func TestImportAccumulatorPreservesUSBHostDeviceMetadata(t *testing.T) {
	t.Parallel()

	acc := newImportAccumulator()
	xml := []byte(`
<domain type='kvm'>
  <name>web</name>
  <devices>
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/web.qcow2'/>
    </disk>
    <hostdev mode='subsystem' type='usb'>
      <source><vendor id='0x0781'/><product id='0x5581'/></source>
    </hostdev>
  </devices>
</domain>
`)

	if err := acc.addDomain("web.xml", xml); err != nil {
		t.Fatalf("addDomain: %v", err)
	}
	file := acc.composeFile("")
	devices := file.Services["web"].Devices
	if len(devices) != 1 || devices[0].Source != "usb:0781:5581" || devices[0].Target != "usb:0781:5581" || devices[0].Permissions != "rwm" {
		t.Fatalf("devices = %+v, want imported USB metadata", devices)
	}
	wantWarning := `web: hostdev usb usb:0781:5581 preserved as device metadata`
	if len(acc.warnings) != 1 || !strings.Contains(acc.warnings[0], wantWarning) {
		t.Fatalf("warnings = %+v, want %q", acc.warnings, wantWarning)
	}
}

func assertImportedComposeYAML(t *testing.T, got string) {
	t.Helper()

	for _, want := range []string{"name: imported", "vm:", "image: debian:13"} {
		if !strings.Contains(got, want) {
			t.Fatalf("imported compose YAML missing %q:\n%s", want, got)
		}
	}
}
