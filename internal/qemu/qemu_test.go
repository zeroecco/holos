package qemu

import (
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func assertArgsContain(t *testing.T, args []string, needles ...string) {
	t.Helper()

	joined := strings.Join(args, " ")
	for _, needle := range needles {
		if !strings.Contains(joined, needle) {
			t.Fatalf("expected args to contain %q, got:\n%s", needle, joined)
		}
	}
}

func assertArgsOmit(t *testing.T, args []string, forbidden ...string) {
	t.Helper()

	joined := strings.Join(args, " ")
	for _, needle := range forbidden {
		if strings.Contains(joined, needle) {
			t.Fatalf("expected args to omit %q, got:\n%s", needle, joined)
		}
	}
}

func TestQEMUOptEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain path", input: "/state/root.qcow2", want: "/state/root.qcow2"},
		{name: "path with comma", input: "/state/weird,path/root", want: "/state/weird,,path/root"},
		{name: "option-looking value", input: "prefix,readonly=off", want: "prefix,,readonly=off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := qemuOptEscape(tt.input); got != tt.want {
				t.Fatalf("qemuOptEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCharDevOptions(t *testing.T) {
	t.Parallel()

	if got, want := consoleCharDevOption("/state/serial,sock", "/state/qemu,log"),
		"socket,id=console0,path=/state/serial,,sock,server=on,wait=off,logfile=/state/qemu,,log,logappend=on"; got != want {
		t.Fatalf("consoleCharDevOption = %q, want %q", got, want)
	}
	if got, want := qmpCharDevOption("/state/qmp,sock"),
		"socket,id=qmp,path=/state/qmp,,sock,server=on,wait=off"; got != want {
		t.Fatalf("qmpCharDevOption = %q, want %q", got, want)
	}
}

func TestBuildArgsIncludesKVMNetworkingAndMounts(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "api",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     2,
			MemoryMB: 1024,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
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
			{Name: "http", HostPort: 8080, GuestPort: 80, Protocol: config.DefaultProtocol},
			{Name: "mdns", HostPort: 5353, GuestPort: 5353, Protocol: config.ProtocolUDP},
			{Name: "admin", HostAddr: "0.0.0.0", HostPort: 9000, GuestAddr: "10.0.2.15", GuestPort: 9000, Protocol: config.DefaultProtocol},
		},
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	assertArgsContain(t, args,
		"-enable-kvm",
		"q35,accel=kvm",
		"net0",
		"virtio-net-pci,netdev=net0",
		"hostfwd=tcp:127.0.0.1:8080-:80",
		"hostfwd=udp:127.0.0.1:5353-:5353",
		"hostfwd=tcp:0.0.0.0:9000-10.0.2.15:9000",
		"-virtfs local,path=/srv/api,mount_tag=share0-var-lib-api,security_model=none,readonly=on",
		"id=root,if=none,cache=writeback,discard=unmap,format=qcow2,file=/state/api-0/root.qcow2",
		"virtio-blk-pci,drive=root,bootindex=1",
		"file=/state/api-0/seed.iso",
	)
	assertArgsOmit(t, args, "net1")
}

func TestBuildArgsOmitsFirmwareUnlessUEFIReady(t *testing.T) {
	t.Parallel()

	readyManifest := config.Manifest{
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 256,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
			UEFI:     true,
		},
	}
	tests := []struct {
		name     string
		manifest config.Manifest
		spec     LaunchSpec
	}{
		{
			name: "uefi disabled",
			manifest: config.Manifest{VM: config.VMConfig{
				VCPU:     1,
				MemoryMB: 256,
				Machine:  config.DefaultMachine,
				CPUModel: config.DefaultCPUModel,
			}},
			spec: LaunchSpec{
				OVMFCode: "/usr/share/OVMF_CODE.fd",
				OVMFVars: "/state/OVMF_VARS.fd",
			},
		},
		{
			name:     "missing code",
			manifest: readyManifest,
			spec:     LaunchSpec{OVMFVars: "/state/OVMF_VARS.fd"},
		},
		{
			name:     "missing vars",
			manifest: readyManifest,
			spec:     LaunchSpec{OVMFCode: "/usr/share/OVMF_CODE.fd"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args, err := BuildArgs(tt.manifest, tt.spec)
			if err != nil {
				t.Fatalf("build args: %v", err)
			}

			assertArgsOmit(t, args, "if=pflash", "OVMF")
		})
	}
}

func TestBuildArgsIncludesFirmwareWhenUEFIReady(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{VM: config.VMConfig{
		VCPU:     1,
		MemoryMB: 256,
		Machine:  config.DefaultMachine,
		CPUModel: config.DefaultCPUModel,
		UEFI:     true,
	}}
	spec := LaunchSpec{
		OVMFCode: "/usr/share/OVMF,variant/OVMF_CODE.fd",
		OVMFVars: "/state/weird,path/OVMF_VARS.fd",
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	assertArgsContain(t, args,
		"if=pflash,format=raw,readonly=on,file=/usr/share/OVMF,,variant/OVMF_CODE.fd",
		"if=pflash,format=raw,file=/state/weird,,path/OVMF_VARS.fd",
	)
}

// TestBuildArgs_EscapesCommasInPaths pins the QEMU option-parser
// contract: a `,` inside any interpolated value must be doubled to
// `,,` before it lands in a comma-delimited option string. QEMU's
// option parser treats `,` as the key=value separator; a directory
// path like `/srv/foo,bar/holos.yaml` without escaping silently
// becomes two pseudo-options (`path=/srv/foo`, `bar/holos.yaml`)
// and QEMU either errors with "unknown parameter" or, worse for a
// -virtfs mount, accepts an attacker-supplied `,readonly=off`
// smuggled into a shared directory name. We exercise every path
// field that flows through BuildArgs: console/qmp chardev sockets,
// overlay/seed/pflash drives, bind-mount sources, and named-volume
// disk paths. If any of these regress to raw interpolation the
// assertion below fails on the very literal substring we just
// injected.
func TestBuildArgs_EscapesCommasInPaths(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "api",
		Image:       "/images/base,v1.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 256,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
			UEFI:     true,
		},
		Mounts: []config.Mount{
			{Source: "/srv/a,b", Target: "/var/lib/x", Kind: config.MountKindBind},
		},
	}

	spec := LaunchSpec{
		Name:        "api-0",
		Index:       0,
		OverlayPath: "/state/weird,path/root.qcow2",
		SeedPath:    "/state/weird,path/seed.iso",
		SerialPath:  "/state/weird,path/serial.sock",
		QMPPath:     "/state/weird,path/qmp.sock",
		LogPath:     "/state/weird,path/console.log",
		OVMFCode:    "/usr/share/OVMF,variant/OVMF_CODE.fd",
		OVMFVars:    "/state/weird,path/OVMF_VARS.fd",
		Volumes: []VolumeAttachment{
			{Name: "data", DiskPath: "/state/vols/a,b.qcow2"},
		},
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	assertArgsContain(t, args,
		"path=/state/weird,,path/serial.sock",
		"logfile=/state/weird,,path/console.log",
		"path=/state/weird,,path/qmp.sock",
		"file=/usr/share/OVMF,,variant/OVMF_CODE.fd",
		"file=/state/weird,,path/OVMF_VARS.fd",
		"file=/state/weird,,path/root.qcow2",
		"file=/state/weird,,path/seed.iso",
		"path=/srv/a,,b",
		"file=/state/vols/a,,b.qcow2",
	)

	// Negative check: no raw single-comma variants of those paths
	// should appear. The double-escape turns a literal comma into
	// ",," so the *single*-comma form is proof of regression.
	assertArgsOmit(t, args,
		"path=/state/weird,path/serial.sock",
		"file=/state/weird,path/root.qcow2",
		"path=/srv/a,b",
		"file=/state/vols/a,b.qcow2",
	)
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
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 256,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
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
	assertArgsContain(t, args, "file=/state/vols/api-data.qcow2,cache=writeback,discard=unmap,readonly=on")
	assertArgsContain(t, args, "virtio-blk-pci,drive=vol-data,serial=vol-data")
	assertArgsOmit(t, args, "file=/state/vols/api-cache.qcow2,cache=writeback,discard=unmap,readonly=on")
}

func TestBuildArgs_RootDiskHasBootIndexWithNamedVolume(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "api",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 256,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
		},
	}
	spec := LaunchSpec{
		Name:        "api-0",
		Index:       0,
		OverlayPath: "/state/api-0/root.qcow2",
		LogPath:     "/state/api-0/console.log",
		QMPPath:     "/state/api-0/qmp.sock",
		Volumes: []VolumeAttachment{
			{Name: "data", DiskPath: "/state/vols/api-data.qcow2"},
		},
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	assertArgsContain(t, args,
		"id=root,if=none,cache=writeback,discard=unmap,format=qcow2,file=/state/api-0/root.qcow2",
		"virtio-blk-pci,drive=root,bootindex=1",
	)
	assertArgsOmit(t, args, "drive=vol-data,serial=vol-data,bootindex=")
}

func TestBuildArgsWithInternalNetwork(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "web",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 512,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: "230.0.0.1",
			MulticastPort:  12345,
			Subnet:         "10.10.0.0/24",
			InstanceIPs:    []string{"10.10.0.2"},
			BaseMAC:        "52:54:00:ab:cd:00",
			UserBaseMAC:    "52:54:01:ab:cd:00",
		},
	}

	spec := LaunchSpec{
		Name:        "web-0",
		Index:       2,
		OverlayPath: "/state/web-0/root.qcow2",
		LogPath:     "/state/web-0/console.log",
		QMPPath:     "/state/web-0/qmp.sock",
	}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}

	assertArgsContain(t, args,
		"net0",
		"net1",
		"socket,id=net1,mcast=230.0.0.1:12345",
		"mcast=230.0.0.1:12345",
		"virtio-net-pci,netdev=net0,mac=52:54:01:ab:cd:02",
		"virtio-net-pci,netdev=net1,mac=52:54:00:ab:cd:02",
	)
}

func TestBuildArgsWithAdditionalInternalNetworkSegments(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "proxy",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 512,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: "230.0.0.1",
			MulticastPort:  12345,
			Subnet:         "10.10.1.0/24",
			InstanceIPs:    []string{"10.10.1.3"},
			BaseMAC:        "52:54:02:ab:cd:00",
			UserBaseMAC:    "52:54:01:ab:cd:00",
			Segments: []config.InternalNetworkSegment{
				{
					Name:           "frontend",
					MulticastGroup: "230.0.0.2",
					MulticastPort:  12346,
					Subnet:         "10.10.2.0/24",
					InstanceIPs:    []string{"10.10.2.3"},
					BaseMAC:        "52:54:02:ef:01:00",
				},
			},
		},
	}
	spec := LaunchSpec{Name: "proxy-0", Index: 0, OverlayPath: "/state/root.qcow2"}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	assertArgsContain(t, args,
		"socket,id=net1,mcast=230.0.0.1:12345",
		"virtio-net-pci,netdev=net1,mac=52:54:02:ab:cd:00",
		"socket,id=net2,mcast=230.0.0.2:12346",
		"virtio-net-pci,netdev=net2,mac=52:54:02:ef:01:00",
	)
}

func TestBuildArgsWithBridgeNetwork(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "web",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     1,
			MemoryMB: 512,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
		},
		InternalNetwork: &config.InternalNetworkConfig{
			BaseMAC:     "52:54:02:ab:cd:00",
			UserBaseMAC: "52:54:01:ab:cd:00",
			Backend:     "bridge",
			BridgeName:  "br0",
		},
	}
	spec := LaunchSpec{Name: "web-0", Index: 0, OverlayPath: "/state/root.qcow2"}

	args, err := BuildArgs(manifest, spec)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	assertArgsContain(t, args,
		"bridge,id=net1,br=br0",
		"virtio-net-pci,netdev=net1,mac=52:54:02:ab:cd:00",
	)
}

func TestBuildArgsWithVFIODevices(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "ml",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     8,
			MemoryMB: 16384,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
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

	assertArgsContain(t, args,
		"kernel-irqchip=on",
		"vfio-pci,host=0000:01:00.0",
		"vfio-pci,host=0000:01:00.1",
		"OVMF_CODE.fd",
		"OVMF_VARS.fd",
	)
}

func TestBuildArgsWithROMFile(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:        "gpu",
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			VCPU:     4,
			MemoryMB: 8192,
			Machine:  config.DefaultMachine,
			CPUModel: config.DefaultCPUModel,
			UEFI:     true,
		},
		Devices: []config.Device{
			{PCI: "0000:41:00.0", ROMFile: "/opt/vbios/gpu,patched.rom"},
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

	assertArgsContain(t, args, "vfio-pci,host=0000:41:00.0,romfile=/opt/vbios/gpu,,patched.rom")
}
