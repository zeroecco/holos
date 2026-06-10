package runtime

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const testQEMUImgLogEnv = "HOLOS_QEMU_IMG_LOG"

func testInstanceWorkDir(index int) (string, string) {
	instanceName := instanceDirName(testPathService, index)
	workDir := projectInstanceDir("/state", testPathProject, testPathService, index)
	return instanceName, workDir
}

func TestStartLaunchSpecUsesInstancePaths(t *testing.T) {
	t.Parallel()

	const instanceIndex = 0

	instanceName, workDir := testInstanceWorkDir(instanceIndex)
	paths := newInstancePaths(workDir)
	volumePath := volumeLinkPath(workDir, testVolumeName)
	volumes := []qemu.VolumeAttachment{{Name: testVolumeName, DiskPath: volumePath}}
	seedPath := newSeedPaths(workDir).isoImage

	spec := startLaunchSpec(instanceName, instanceIndex, paths, seedPath, volumes)
	assertLaunchSpecIdentity(t, spec, instanceName, instanceIndex)
	assertLaunchSpecPaths(t, spec, paths.overlay, seedPath, paths.consoleLog, paths.serialSocket, paths.qmpSocket)
	assertLaunchSpecVolumes(t, spec, volumes)
}

func TestInspectLaunchSpecReconstructsSavedInstance(t *testing.T) {
	t.Parallel()

	instancePorts := []qemu.PortMapping{{HostPort: 8080, GuestPort: 80, Protocol: config.ProtocolTCP}}
	workDir := filepath.Join("/state/instances", testPathProject, testPathService, testInstanceName(testPathService, 0))
	instance := InstanceRecord{
		Name:        testInstanceName(testPathService, 0),
		Index:       0,
		WorkDir:     workDir,
		OverlayPath: filepath.Join(workDir, "root.qcow2"),
		SeedPath:    filepath.Join(workDir, "seed.iso"),
		LogPath:     filepath.Join(workDir, "console.log"),
		SerialPath:  filepath.Join(workDir, "serial.sock"),
		QMPPath:     filepath.Join(workDir, "qmp.sock"),
		Ports:       instancePorts,
		SSHPort:     2222,
	}
	manifest := config.Manifest{
		VM: config.VMConfig{UEFI: true},
		Mounts: []config.Mount{
			{Kind: config.MountKindBind, Target: "/srv"},
			{Kind: config.MountKindVolume, VolumeName: testVolumeName, ReadOnly: true},
		},
	}

	spec := InspectLaunchSpec(manifest, instance)

	assertLaunchSpecIdentity(t, spec, instance.Name, instance.Index)
	assertLaunchSpecPaths(t, spec, instance.OverlayPath, instance.SeedPath, instance.LogPath, instance.SerialPath, instance.QMPPath)
	if !slices.Equal(spec.Ports, instance.Ports) || spec.SSHPort != instance.SSHPort {
		t.Fatalf("ports = %+v ssh=%d, want ports=%+v ssh=%d", spec.Ports, spec.SSHPort, instance.Ports, instance.SSHPort)
	}
	assertLaunchSpecVolumes(t, spec, []qemu.VolumeAttachment{{
		Name:     testVolumeName,
		DiskPath: filepath.Join(workDir, "vol-"+testVolumeName+".qcow2"),
		ReadOnly: true,
	}})
	if spec.OVMFVars != filepath.Join(workDir, "OVMF_VARS.fd") {
		t.Fatalf("OVMFVars = %q, want workdir OVMF vars", spec.OVMFVars)
	}
}

func TestStartedInstanceRecordUsesLaunchResultAndInstancePaths(t *testing.T) {
	t.Parallel()

	const instanceIndex = 0

	instanceName, workDir := testInstanceWorkDir(instanceIndex)
	paths := newInstancePaths(workDir)
	seedPath := newSeedPaths(workDir).isoImage
	startedAt := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)
	ports := testHTTPPortMappings()
	manifest := config.Manifest{StopGracePeriodSec: 9}

	record := startedInstanceRecord(instanceName, instanceIndex, workDir, paths, seedPath, manifest, 1234, ports, 2222, nil, startedAt)
	assertInstanceRecordIdentity(t, record, instanceName, instanceIndex, workDir)
	assertInstanceRecordRunning(t, record, 1234)
	assertInstanceRecordPaths(t, record, paths.overlay, seedPath, paths.consoleLog, paths.serialSocket, paths.qmpSocket)
	assertInstanceRecordPorts(t, record, ports, 2222)
	assertInstanceRecordTiming(t, record, 9, startedAt)
}

func TestInstanceRecordWithLaunchResult(t *testing.T) {
	t.Parallel()

	const instanceIndex = 0

	instanceName, workDir := testInstanceWorkDir(instanceIndex)
	startedAt := time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC)
	ports := testHTTPPortMappings()
	base := InstanceRecord{
		Name:    instanceName,
		Index:   instanceIndex,
		WorkDir: workDir,
	}

	record := instanceRecordWithLaunchResult(base, 1234, ports, 2222, startedAt)
	assertInstanceRecordIdentity(t, record, instanceName, instanceIndex, workDir)
	assertInstanceRecordRunning(t, record, 1234)
	assertInstanceRecordPorts(t, record, ports, 2222)
	if !record.LastStarted.Equal(startedAt) {
		t.Fatalf("LastStarted = %s, want %s", record.LastStarted, startedAt)
	}
}

func testHTTPPortMappings() []qemu.PortMapping {
	return []qemu.PortMapping{{Name: "http", HostPort: 8080, GuestPort: 80, Protocol: config.DefaultProtocol}}
}

func TestCreateOverlayIncludesConfiguredDiskSize(t *testing.T) {
	dir := t.TempDir()
	qemuImg := filepath.Join(dir, qemuImgDefault)
	logPath := filepath.Join(dir, qemuImgDefault+".log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$" + testQEMUImgLogEnv + "\"\n"
	if err := os.WriteFile(qemuImg, []byte(script), 0o755); err != nil {
		t.Fatalf("write qemu-img mock: %v", err)
	}
	t.Setenv(qemuImgEnv, qemuImg)
	t.Setenv(testQEMUImgLogEnv, logPath)

	m := &Manager{}
	manifest := config.Manifest{
		Image:       "/images/base.qcow2",
		ImageFormat: config.ImageFormatQCOW2,
		VM: config.VMConfig{
			DiskSizeBytes: 2 * (1 << 30),
		},
	}
	overlayPath := filepath.Join(dir, instanceOverlayFilename)
	if err := m.createOverlay(manifest, overlayPath); err != nil {
		t.Fatalf("createOverlay: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"create",
		"-f", "qcow2",
		"-F", "qcow2",
		"-b", "/images/base.qcow2",
		overlayPath,
		"2147483648",
	}
	assertStringSliceEqual(t, "qemu-img args", args, want)
}

func TestOverlayCreateArgsOmitsDiskSizeWhenUnset(t *testing.T) {
	t.Parallel()

	imageName := "base." + config.ImageFormatRaw
	imagePath := filepath.Join("/images", imageName)
	_, workDir := testInstanceWorkDir(0)
	overlayPath := filepath.Join(workDir, instanceOverlayFilename)
	manifest := config.Manifest{
		Image:       imagePath,
		ImageFormat: config.ImageFormatRaw,
	}
	got := overlayCreateArgs(manifest, overlayPath)
	want := []string{
		"create",
		"-f", "qcow2",
		"-F", "raw",
		"-b", imagePath,
		overlayPath,
	}
	assertStringSliceEqual(t, "overlayCreateArgs", got, want)
}
