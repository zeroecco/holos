package qemu

import (
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestBuildArgsIncludesKVMNetworkingAndMounts(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "api",
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			VCPU:     2,
			MemoryMB: 1024,
			Machine:  "q35",
			CPUModel: "host",
		},
		Mounts: []config.Mount{
			{Source: "/srv/api", Target: "/var/lib/api", ReadOnly: true},
		},
	}

	spec := LaunchSpec{
		Name:        "api-0",
		Index:       0,
		OverlayPath: "/state/api-0/root.qcow2",
		SeedPath:    "/state/api-0/seed.iso",
		LogPath:     "/state/api-0/console.log",
		QMPPath:     "/state/api-0/qmp.sock",
		Ports: []PortMapping{
			{Name: "http", HostPort: 8080, GuestPort: 80, Protocol: "tcp"},
		},
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	joined := strings.Join(args, " ")
	for _, needle := range []string{
		"-enable-kvm",
		"q35,accel=kvm",
		"hostfwd=tcp:127.0.0.1:8080-:80",
		"-virtfs local,path=/srv/api,mount_tag=share0-var-lib-api,security_model=none,readonly=on",
		"file=/state/api-0/root.qcow2",
		"file=/state/api-0/seed.iso",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("expected args to contain %q, got:\n%s", needle, joined)
		}
	}
}

// TestBuildArgs_NamedVolumeReadOnly pins the `:ro` flag through to
// QEMU. Prior to the fix the compose parser recorded ReadOnly, the
// runtime dropped it in materializeInstanceVolumes, and the guest got
// a writable disk despite the operator's explicit request. A broken
// read-only contract lets one VM trash a shared backing qcow2, so we
// assert readonly=on lands on the -drive node for the named-volume
// path but does *not* appear on writable volumes in the same launch.
func TestBuildArgs_NamedVolumeReadOnly(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "api",
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 256,
			Machine:  "q35",
			CPUModel: "host",
		},
	}
	spec := LaunchSpec{
		Name:        "api-0",
		Index:       0,
		OverlayPath: "/state/api-0/root.qcow2",
		SeedPath:    "/state/api-0/seed.iso",
		LogPath:     "/state/api-0/console.log",
		QMPPath:     "/state/api-0/qmp.sock",
		Volumes: []VolumeAttachment{
			{Name: "data", DiskPath: "/state/vols/api-data.qcow2", ReadOnly: true},
			{Name: "cache", DiskPath: "/state/vols/api-cache.qcow2"},
		},
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "file=/state/vols/api-data.qcow2,cache=writeback,discard=unmap,readonly=on") {
		t.Fatalf("read-only volume lost readonly=on: %s", joined)
	}
	if strings.Contains(joined, "file=/state/vols/api-cache.qcow2,cache=writeback,discard=unmap,readonly=on") {
		t.Fatalf("writable volume unexpectedly got readonly=on: %s", joined)
	}
}

func TestBuildArgsWithInternalNetwork(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "web",
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 512,
			Machine:  "q35",
			CPUModel: "host",
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: "230.0.0.1",
			MulticastPort:  12345,
			Subnet:         "10.10.0.0/24",
			InstanceIPs:    []string{"10.10.0.2"},
			BaseMAC:        "52:54:00:ab:cd:00",
		},
	}

	spec := LaunchSpec{
		Name:        "web-0",
		Index:       0,
		OverlayPath: "/state/web-0/root.qcow2",
		LogPath:     "/state/web-0/console.log",
		QMPPath:     "/state/web-0/qmp.sock",
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "net0") {
		t.Fatal("expected user-mode netdev net0")
	}
	if !strings.Contains(joined, "net1") {
		t.Fatal("expected socket netdev net1")
	}
	if !strings.Contains(joined, "mcast=230.0.0.1:12345") {
		t.Fatalf("expected multicast in args:\n%s", joined)
	}
	if !strings.Contains(joined, "mac=52:54:00:ab:cd:00") {
		t.Fatalf("expected MAC in args:\n%s", joined)
	}
}

func TestBuildArgsWithVFIODevices(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "ml",
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			VCPU:     8,
			MemoryMB: 16384,
			Machine:  "q35",
			CPUModel: "host",
			UEFI:     true,
		},
		Devices: []config.Device{
			{PCI: "0000:01:00.0"},
			{PCI: "0000:01:00.1"},
		},
	}

	spec := LaunchSpec{
		Name:        "ml-0",
		Index:       0,
		OverlayPath: "/state/ml-0/root.qcow2",
		LogPath:     "/state/ml-0/console.log",
		QMPPath:     "/state/ml-0/qmp.sock",
		OVMFCode:    "/usr/share/OVMF/OVMF_CODE.fd",
		OVMFVars:    "/state/ml-0/OVMF_VARS.fd",
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	joined := strings.Join(args, " ")

	for _, needle := range []string{
		"kernel-irqchip=on",
		"vfio-pci,host=0000:01:00.0",
		"vfio-pci,host=0000:01:00.1",
		"OVMF_CODE.fd",
		"OVMF_VARS.fd",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("expected args to contain %q, got:\n%s", needle, joined)
		}
	}
}

func TestBuildArgsWithROMFile(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "gpu",
		Image:       "/images/base.qcow2",
		ImageFormat: "qcow2",
		VM: config.VMConfig{
			VCPU:     4,
			MemoryMB: 8192,
			Machine:  "q35",
			CPUModel: "host",
			UEFI:     true,
		},
		Devices: []config.Device{
			{PCI: "0000:41:00.0", ROMFile: "/opt/vbios/gpu.rom"},
		},
	}

	spec := LaunchSpec{
		Name:        "gpu-0",
		Index:       0,
		OverlayPath: "/state/gpu-0/root.qcow2",
		LogPath:     "/state/gpu-0/console.log",
		QMPPath:     "/state/gpu-0/qmp.sock",
		OVMFCode:    "/usr/share/OVMF/OVMF_CODE.fd",
		OVMFVars:    "/state/gpu-0/OVMF_VARS.fd",
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "romfile=/opt/vbios/gpu.rom") {
		t.Fatalf("expected romfile in args:\n%s", joined)
	}
}
