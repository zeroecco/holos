package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testManifestImage       = "/tmp/base.qcow2"
	testManifestAPIService  = "api"
	testManifestGPUService  = "gpu"
	testInvalidCloudUser    = "bad user"
	testLoopbackHostAddr    = "127.0.0.1"
	testAltLoopbackHostAddr = "127.0.0.2"
	testWildcardHostAddr    = "0.0.0.0"
	testInvalidHostAddr     = "localhost"
	testGuestAddr           = "10.0.2.15"
	testInvalidGuestAddr    = "::1"
	testHostPort            = 8080
	testGuestPort           = 80
	testAltGuestPort        = 81
	testHealthcheckCmd      = "CMD"
	testHealthcheckArg      = "true"
	testExplicitIntervalSec = 2
	testExplicitRetries     = 4
	testExplicitStartSec    = 6
	testExplicitStartIntSec = 3
	testExplicitTimeoutSec  = 8
	testUDPProtocol         = "udp"
	testBindSource          = "/host"
	testBindTarget          = "/guest"
	testVolumeMountName     = "data"
	testVolumeTarget        = "/data"
	testUnknownMountKind    = "tmpfs"
	testUnknownMountTarget  = "/tmp"
	testExplicitFilePerm    = "0600"
	testExplicitFileOwner   = "app:app"
)

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err.Error(), want)
	}
}

func assertWriteFileMetadata(t *testing.T, name string, got WriteFile, permissions, owner string) {
	t.Helper()

	if got.Permissions != permissions || got.Owner != owner {
		t.Fatalf("%s metadata = %+v, want permissions %q owner %q", name, got, permissions, owner)
	}
}

func validTestManifest(name string) Manifest {
	return Manifest{
		Name:        name,
		Replicas:    1,
		Image:       testManifestImage,
		ImageFormat: ImageFormatQCOW2,
		VM:          VMConfig{VCPU: 1, MemoryMB: 512},
		Network:     NetworkConfig{Mode: DefaultNetworkMode},
		CloudInit:   CloudInit{User: DefaultUser},
	}
}

func TestLoadManifestDefaultsAndPathResolution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	manifestPath := filepath.Join(root, "service.json")
	content := `{
  "name": "api",
  "image": "./images/base.qcow2",
  "ports": [{"guest_port": 8080}],
  "mounts": [{"source": "./data", "target": "/var/lib/api"}],
  "cloud_init": {
    "write_files": [{"path": "/etc/api.env", "content": "MODE=prod\n"}]
  }
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if manifest.APIVersion != DefaultAPIVersion {
		t.Fatalf("unexpected api version: %s", manifest.APIVersion)
	}
	if manifest.Replicas != DefaultReplicas {
		t.Fatalf("unexpected replicas: %d", manifest.Replicas)
	}
	if manifest.Ports[0].Protocol != DefaultProtocol {
		t.Fatalf("unexpected protocol: %s", manifest.Ports[0].Protocol)
	}
	if manifest.CloudInit.User != DefaultUser {
		t.Fatalf("unexpected cloud-init user: %s", manifest.CloudInit.User)
	}
	if !filepath.IsAbs(manifest.Image) {
		t.Fatalf("expected absolute image path, got %s", manifest.Image)
	}
	if !filepath.IsAbs(manifest.Mounts[0].Source) {
		t.Fatalf("expected absolute mount source, got %s", manifest.Mounts[0].Source)
	}
}

func TestLoadManifestRejectsInvalidServiceName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "service.json")
	content := `{"name": "INVALID_NAME", "image": "/tmp/base.qcow2"}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := LoadManifest(manifestPath); err == nil {
		t.Fatal("expected invalid service name error")
	}
}

func TestValidateRejectsInvalidCloudInitUser(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestAPIService)
	m.CloudInit.User = testInvalidCloudUser
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid cloud_init.user error")
	}
}

func TestValidateRejectsInvalidPCIAddress(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestGPUService)
	m.Devices = []Device{{PCI: "01:00.8"}}
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid PCI address error")
	}
}

func TestValidateRejectsTinyDiskSize(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestAPIService)
	m.VM.DiskSizeBytes = 100
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid vm.disk_size_bytes error")
	}
}

func TestValidateManifestMinimums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Manifest)
		wantErr string
	}{
		{
			name:    "replicas",
			mutate:  func(m *Manifest) { m.Replicas = minManifestReplicas - 1 },
			wantErr: "replicas must be >= 1",
		},
		{
			name:    "vcpu",
			mutate:  func(m *Manifest) { m.VM.VCPU = minManifestVCPU - 1 },
			wantErr: "vm.vcpu must be >= 1",
		},
		{
			name:    "memory",
			mutate:  func(m *Manifest) { m.VM.MemoryMB = minManifestMemoryMB - 1 },
			wantErr: "vm.memory_mb must be >= 128",
		},
		{
			name:    "stop grace",
			mutate:  func(m *Manifest) { m.StopGracePeriodSec = minStopGracePeriodSec - 1 },
			wantErr: "stop_grace_period_sec must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manifest := validTestManifest(testManifestAPIService)
			tt.mutate(&manifest)
			assertErrorContains(t, manifest.Validate(), tt.wantErr)
		})
	}
}

func TestValidateRejectsUnsupportedImageFormat(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestAPIService)
	m.ImageFormat = "vmdk"
	assertErrorContains(t, m.Validate(), "image_format must be one of")
}

func TestValidateRejectsUnsupportedImageOS(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestAPIService)
	m.ImageOS = "sysvinit"
	assertErrorContains(t, m.Validate(), "image_os must be one of")
}

func TestValidateRejectsUnsupportedNetworkMode(t *testing.T) {
	t.Parallel()

	m := validTestManifest(testManifestAPIService)
	m.Network.Mode = "bridge"
	assertErrorContains(t, m.Validate(), `network.mode "bridge" is unsupported`)
}

func TestValidateHealthcheck(t *testing.T) {
	t.Parallel()

	if err := validateHealthcheck(nil); err != nil {
		t.Fatalf("validateHealthcheck(nil): %v", err)
	}
}

func TestValidateHealthcheckConfig(t *testing.T) {
	t.Parallel()

	valid := HealthcheckConfig{
		Test:             []string{testHealthcheckCmd, testHealthcheckArg},
		IntervalSec:      minHealthcheckIntervalSec,
		Retries:          minHealthcheckRetries,
		TimeoutSec:       minHealthcheckTimeoutSec,
		StartPeriodSec:   minHealthcheckStartPeriodSec,
		StartIntervalSec: minHealthcheckIntervalSec,
	}

	tests := []struct {
		name    string
		mutate  func(*HealthcheckConfig)
		wantErr string
	}{
		{name: "valid"},
		{name: "missing test", mutate: func(h *HealthcheckConfig) { h.Test = nil }, wantErr: "healthcheck.test is required"},
		{
			name:    "zero interval",
			mutate:  func(h *HealthcheckConfig) { h.IntervalSec = minHealthcheckIntervalSec - 1 },
			wantErr: "healthcheck.interval_sec must be >= 1",
		},
		{
			name:    "zero retries",
			mutate:  func(h *HealthcheckConfig) { h.Retries = minHealthcheckRetries - 1 },
			wantErr: "healthcheck.retries must be >= 1",
		},
		{
			name:    "zero timeout",
			mutate:  func(h *HealthcheckConfig) { h.TimeoutSec = minHealthcheckTimeoutSec - 1 },
			wantErr: "healthcheck.timeout_sec must be >= 1",
		},
		{
			name:    "negative start period",
			mutate:  func(h *HealthcheckConfig) { h.StartPeriodSec = minHealthcheckStartPeriodSec - 1 },
			wantErr: "healthcheck.start_period_sec must be >= 0",
		},
		{
			name:   "zero start interval is fallback",
			mutate: func(h *HealthcheckConfig) { h.StartIntervalSec = 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			healthcheck := valid
			if tt.mutate != nil {
				tt.mutate(&healthcheck)
			}

			err := validateHealthcheckConfig(healthcheck)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateHealthcheckConfig: %v", err)
			}
		})
	}
}

func TestValidatePortAddresses(t *testing.T) {
	t.Parallel()

	base := validTestManifest(testManifestAPIService)
	base.Ports = []PortForward{
		{
			HostAddr:  testLoopbackHostAddr,
			HostPort:  testHostPort,
			GuestAddr: testGuestAddr,
			GuestPort: testGuestPort,
			Protocol:  DefaultProtocol,
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() with explicit addresses: %v", err)
	}

	invalidHost := base
	invalidHost.Ports = []PortForward{
		{
			HostAddr:  testInvalidHostAddr,
			HostPort:  testHostPort,
			GuestPort: testGuestPort,
			Protocol:  DefaultProtocol,
		},
	}
	if err := invalidHost.Validate(); err == nil {
		t.Fatal("expected invalid host address error")
	}

	invalidGuest := base
	invalidGuest.Ports = []PortForward{
		{
			HostAddr:  testLoopbackHostAddr,
			HostPort:  testHostPort,
			GuestAddr: testInvalidGuestAddr,
			GuestPort: testGuestPort,
			Protocol:  DefaultProtocol,
		},
	}
	if err := invalidGuest.Validate(); err == nil {
		t.Fatal("expected invalid guest address error")
	}
}

func TestValidatePortAddressConflicts(t *testing.T) {
	t.Parallel()

	base := validTestManifest(testManifestAPIService)

	samePortDifferentLoopback := base
	samePortDifferentLoopback.Ports = []PortForward{
		{
			HostAddr:  testLoopbackHostAddr,
			HostPort:  testHostPort,
			GuestPort: testGuestPort,
			Protocol:  DefaultProtocol,
		},
		{
			HostAddr:  testAltLoopbackHostAddr,
			HostPort:  testHostPort,
			GuestPort: testAltGuestPort,
			Protocol:  DefaultProtocol,
		},
	}
	if err := samePortDifferentLoopback.Validate(); err != nil {
		t.Fatalf("different host addresses should not conflict: %v", err)
	}

	wildcardConflict := base
	wildcardConflict.Ports = []PortForward{
		{
			HostAddr:  testWildcardHostAddr,
			HostPort:  testHostPort,
			GuestPort: testGuestPort,
			Protocol:  DefaultProtocol,
		},
		{
			HostAddr:  testLoopbackHostAddr,
			HostPort:  testHostPort,
			GuestPort: testAltGuestPort,
			Protocol:  DefaultProtocol,
		},
	}
	if err := wildcardConflict.Validate(); err == nil {
		t.Fatal("expected wildcard host address conflict")
	}

	samePortDifferentProtocol := base
	samePortDifferentProtocol.Ports = []PortForward{
		{
			HostPort:  testHostPort,
			GuestPort: testGuestPort,
			Protocol:  ProtocolTCP,
		},
		{
			HostPort:  testHostPort,
			GuestPort: testAltGuestPort,
			Protocol:  ProtocolUDP,
		},
	}
	if err := samePortDifferentProtocol.Validate(); err != nil {
		t.Fatalf("same host port with tcp and udp should not conflict: %v", err)
	}
}

func TestApplyHealthcheckDefaults(t *testing.T) {
	t.Parallel()

	healthcheck := HealthcheckConfig{Test: []string{testHealthcheckCmd, testHealthcheckArg}}
	applyHealthcheckDefaults(&healthcheck)
	if healthcheck.IntervalSec != DefaultHealthIntervalSec {
		t.Fatalf("IntervalSec = %d, want %d", healthcheck.IntervalSec, DefaultHealthIntervalSec)
	}
	if healthcheck.Retries != DefaultHealthRetries {
		t.Fatalf("Retries = %d, want %d", healthcheck.Retries, DefaultHealthRetries)
	}
	if healthcheck.TimeoutSec != DefaultHealthTimeoutSec {
		t.Fatalf("TimeoutSec = %d, want %d", healthcheck.TimeoutSec, DefaultHealthTimeoutSec)
	}
	if healthcheck.StartIntervalSec != DefaultHealthIntervalSec {
		t.Fatalf("StartIntervalSec = %d, want interval default %d", healthcheck.StartIntervalSec, DefaultHealthIntervalSec)
	}

	explicit := HealthcheckConfig{
		Test:             []string{testHealthcheckCmd, testHealthcheckArg},
		IntervalSec:      testExplicitIntervalSec,
		Retries:          testExplicitRetries,
		StartPeriodSec:   testExplicitStartSec,
		StartIntervalSec: testExplicitStartIntSec,
		TimeoutSec:       testExplicitTimeoutSec,
	}
	applyHealthcheckDefaults(&explicit)
	if explicit.IntervalSec != testExplicitIntervalSec ||
		explicit.Retries != testExplicitRetries ||
		explicit.StartPeriodSec != testExplicitStartSec ||
		explicit.StartIntervalSec != testExplicitStartIntSec ||
		explicit.TimeoutSec != testExplicitTimeoutSec {
		t.Fatalf("explicit healthcheck defaults overwritten: %+v", explicit)
	}
}

func TestIntOrDefault(t *testing.T) {
	t.Parallel()

	if got := intOrDefault(0, 5); got != 5 {
		t.Fatalf("intOrDefault(zero) = %d, want 5", got)
	}
	if got := intOrDefault(3, 5); got != 3 {
		t.Fatalf("intOrDefault(explicit) = %d, want 3", got)
	}
}

func TestApplyPortDefaults(t *testing.T) {
	t.Parallel()

	port := PortForward{GuestPort: testGuestPort}
	applyPortDefaults(&port)
	if port.Protocol != DefaultProtocol {
		t.Fatalf("Protocol = %q, want %q", port.Protocol, DefaultProtocol)
	}

	explicit := PortForward{GuestPort: testGuestPort, Protocol: testUDPProtocol}
	applyPortDefaults(&explicit)
	if explicit.Protocol != testUDPProtocol {
		t.Fatalf("explicit Protocol overwritten with %q", explicit.Protocol)
	}
}

func TestValidateMount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mount   Mount
		wantErr string
	}{
		{name: "default bind", mount: Mount{Source: testBindSource, Target: testBindTarget}},
		{name: "explicit bind", mount: Mount{Kind: MountKindBind, Source: testBindSource, Target: testBindTarget}},
		{name: "volume", mount: Mount{Kind: MountKindVolume, VolumeName: testVolumeMountName, SizeBytes: minVolumeSizeBytes, Target: testVolumeTarget}},
		{name: "missing target", mount: Mount{Source: testBindSource}, wantErr: "mounts require target"},
		{name: "bind missing source", mount: Mount{Target: testBindTarget}, wantErr: `bind mount "/guest" requires source`},
		{name: "volume missing name", mount: Mount{Kind: MountKindVolume, SizeBytes: minVolumeSizeBytes, Target: testVolumeTarget}, wantErr: `volume mount "/data" requires volume_name`},
		{name: "volume too small", mount: Mount{Kind: MountKindVolume, VolumeName: testVolumeMountName, SizeBytes: minVolumeSizeBytes - 1, Target: testVolumeTarget}, wantErr: "below minimum"},
		{name: "unknown kind", mount: Mount{Kind: testUnknownMountKind, Target: testUnknownMountTarget}, wantErr: `mount "/tmp": unknown kind "tmpfs"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateMount(tt.mount)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateMount: %v", err)
			}
		})
	}
}

func TestApplyMountDefaults(t *testing.T) {
	t.Parallel()

	mount := Mount{Target: testVolumeTarget}
	applyMountDefaults(&mount)
	if mount.Kind != MountKindBind {
		t.Fatalf("Kind = %q, want %q", mount.Kind, MountKindBind)
	}

	for _, kind := range []string{MountKindBind, MountKindVolume} {
		explicit := Mount{Kind: kind, Target: testVolumeTarget}
		applyMountDefaults(&explicit)
		if explicit.Kind != kind {
			t.Fatalf("explicit Kind %q overwritten with %q", kind, explicit.Kind)
		}
	}
}

func TestValidateVolumeMount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mount   Mount
		wantErr string
	}{
		{name: "valid", mount: Mount{VolumeName: testVolumeMountName, SizeBytes: minVolumeSizeBytes, Target: testVolumeTarget}},
		{name: "missing name", mount: Mount{SizeBytes: minVolumeSizeBytes, Target: testVolumeTarget}, wantErr: "requires volume_name"},
		{name: "too small", mount: Mount{VolumeName: testVolumeMountName, SizeBytes: minVolumeSizeBytes - 1, Target: testVolumeTarget}, wantErr: "below minimum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateVolumeMount(tt.mount)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateVolumeMount: %v", err)
			}
		})
	}
}

func TestValidateWriteFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    WriteFile
		wantErr string
	}{
		{name: "valid", file: WriteFile{Path: "/etc/app.conf", Content: "enabled=true\n"}},
		{name: "missing path", file: WriteFile{Content: "enabled=true\n"}, wantErr: "write_files entries require path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateWriteFile(tt.file)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateWriteFile: %v", err)
			}
		})
	}
}

func TestApplyWriteFileDefaults(t *testing.T) {
	t.Parallel()

	file := WriteFile{Path: "/etc/app.conf"}
	applyWriteFileDefaults(&file)
	assertWriteFileMetadata(t, "default write file", file, DefaultFilePermissions, DefaultFileOwner)

	explicit := WriteFile{
		Path:        "/etc/secret",
		Permissions: testExplicitFilePerm,
		Owner:       testExplicitFileOwner,
	}
	applyWriteFileDefaults(&explicit)
	assertWriteFileMetadata(t, "explicit write file", explicit, testExplicitFilePerm, testExplicitFileOwner)
}

func TestValidateUserName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"ubuntu", "_svc", "build-user", "u123"} {
		if err := ValidateUserName(name); err != nil {
			t.Fatalf("ValidateUserName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "Ubuntu", "123user", testInvalidCloudUser, "bad/user", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if err := ValidateUserName(name); err == nil {
			t.Fatalf("ValidateUserName(%q) succeeded, want error", name)
		}
	}
}

func TestValidatePCIAddress(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"0000:01:00.0", "abcd:ef:12.7"} {
		if err := ValidatePCIAddress(addr); err != nil {
			t.Fatalf("ValidatePCIAddress(%q): %v", addr, err)
		}
	}
	for _, addr := range []string{"", "01:00.0", "0000:01:00.8", "0000:01:00", "0000:1:00.0"} {
		if err := ValidatePCIAddress(addr); err == nil {
			t.Fatalf("ValidatePCIAddress(%q) succeeded, want error", addr)
		}
	}
}

func TestValidateMACAddress(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"02:42:ac:11:00:02", "52:54:00:ab:cd:ef", "AA:BB:CC:DD:EE:F0"} {
		if err := ValidateMACAddress(addr); err != nil {
			t.Fatalf("ValidateMACAddress(%q): %v", addr, err)
		}
	}
	for _, addr := range []string{"", "02:42:ac:11:00", "02-42-ac-11-00-02", "01:42:ac:11:00:02", "ff:ff:ff:ff:ff:ff"} {
		if err := ValidateMACAddress(addr); err == nil {
			t.Fatalf("ValidateMACAddress(%q) succeeded, want error", addr)
		}
	}
}

func TestEmptyScalar(t *testing.T) {
	t.Parallel()

	if !emptyScalar("") {
		t.Fatal("emptyScalar(empty) = false, want true")
	}
	if emptyScalar("value") {
		t.Fatal("emptyScalar(value) = true, want false")
	}
}

func TestInternalNetworkIdentity(t *testing.T) {
	t.Parallel()

	network := InternalNetworkConfig{
		BaseMAC:     "02:00:00:00:00:10",
		UserBaseMAC: "02:00:00:00:01:20",
		InstanceIPs: []string{
			"10.44.0.10",
			"10.44.0.11",
		},
	}
	if got := network.InstanceMAC(2); got != "02:00:00:00:00:12" {
		t.Fatalf("InstanceMAC(2) = %q", got)
	}
	if got := network.UserMAC(3); got != "02:00:00:00:01:23" {
		t.Fatalf("UserMAC(3) = %q", got)
	}
	if got := network.InstanceIP(1); got != "10.44.0.11" {
		t.Fatalf("InstanceIP(1) = %q", got)
	}
	if got := network.InstanceIP(2); got != "" {
		t.Fatalf("InstanceIP(2) = %q, want empty", got)
	}
}

func TestOffsetMACKeepsMalformedBase(t *testing.T) {
	t.Parallel()

	for _, base := range []string{
		"not-a-mac",
		"02:00:00:00:00:xx",
	} {
		if got := offsetMAC(base, 3); got != base {
			t.Fatalf("offsetMAC(%q) = %q, want original base", base, got)
		}
	}
}

func TestOffsetMACWrapsLastOctet(t *testing.T) {
	t.Parallel()

	if got := offsetMAC("02:00:00:00:00:ff", 1); got != "02:00:00:00:00:00" {
		t.Fatalf("offsetMAC wrap = %q, want 02:00:00:00:00:00", got)
	}
}
