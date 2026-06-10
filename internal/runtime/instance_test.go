package runtime

import (
	"os"
	"path/filepath"
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

func TestStartedInstanceRecordUsesLaunchResultAndInstancePaths(t *testing.T) {
	t.Parallel()

	const instanceIndex = 0

	instanceName, workDir := testInstanceWorkDir(instanceIndex)
	paths := newInstancePaths(workDir)
	seedPath := newSeedPaths(workDir).isoImage
	startedAt := time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC)
	ports := testHTTPPortMappings()
	manifest := config.Manifest{StopGracePeriodSec: 9}

	record := startedInstanceRecord(instanceName, instanceIndex, workDir, paths, seedPath, manifest, 1234, ports, 2222, startedAt)
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
