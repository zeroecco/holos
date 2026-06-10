package runtime

import (
	"net"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	testRestartLaunchIndex = 2
	testRestartRecordIndex = 1
)

func TestRestartLaunchSpecCarriesPreviousPaths(t *testing.T) {
	t.Parallel()

	instanceName, workDir := testInstanceWorkDir(0)
	volumePath := volumeLinkPath(workDir, testVolumeName)
	volumes := []qemu.VolumeAttachment{{Name: testVolumeName, DiskPath: volumePath, ReadOnly: true}}
	prev := testPreviousInstanceRecord(instanceName, testRestartLaunchIndex, workDir)

	spec := restartLaunchSpec(prev, volumes)
	assertLaunchSpecIdentity(t, spec, prev.Name, prev.Index)
	assertLaunchSpecPaths(t, spec, prev.OverlayPath, prev.SeedPath, prev.LogPath, prev.SerialPath, prev.QMPPath)
	assertLaunchSpecVolumes(t, spec, volumes)
}

func TestRestartedInstanceRecordCarriesPreviousStateAndLaunchResult(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	ports := testHTTPPortMappings()
	instanceName, workDir := testInstanceWorkDir(0)
	prev := testPreviousInstanceRecord(instanceName, testRestartRecordIndex, workDir)
	manifest := config.Manifest{StopGracePeriodSec: 12}

	taps := map[string]string{"net1": "htabcdef123456"}
	record := restartedInstanceRecord(prev, manifest, 4321, ports, 2222, taps, startedAt)
	assertInstanceRecordIdentity(t, record, prev.Name, prev.Index, prev.WorkDir)
	assertInstanceRecordRunning(t, record, 4321)
	assertInstanceRecordPaths(t, record, prev.OverlayPath, prev.SeedPath, prev.LogPath, prev.SerialPath, prev.QMPPath)
	assertInstanceRecordPorts(t, record, ports, 2222)
	if record.TapIfNames["net1"] != taps["net1"] {
		t.Fatalf("TapIfNames = %#v, want %#v", record.TapIfNames, taps)
	}
	assertInstanceRecordTiming(t, record, 12, startedAt)
}

func testPreviousInstanceRecord(name string, index int, workDir string) InstanceRecord {
	paths := newInstancePaths(workDir)
	return InstanceRecord{
		Name:        name,
		Index:       index,
		WorkDir:     workDir,
		OverlayPath: paths.overlay,
		SeedPath:    newSeedPaths(workDir).isoImage,
		LogPath:     paths.consoleLog,
		SerialPath:  paths.serialSocket,
		QMPPath:     paths.qmpSocket,
	}
}

func TestShouldAllocateRestartSSHPort(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen(tcpNetwork, net.JoinHostPort(defaultHostAddr, ephemeralPortSpec))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	busyPort := listener.Addr().(*net.TCPAddr).Port

	availableListener, err := net.Listen(tcpNetwork, net.JoinHostPort(defaultHostAddr, ephemeralPortSpec))
	if err != nil {
		t.Fatalf("listen available candidate: %v", err)
	}
	availablePort := availableListener.Addr().(*net.TCPAddr).Port
	if err := availableListener.Close(); err != nil {
		t.Fatalf("close available candidate: %v", err)
	}

	tests := []struct {
		name         string
		firstAttempt bool
		previousPort int
		want         bool
	}{
		{name: "retry attempt", firstAttempt: false, previousPort: availablePort, want: true},
		{name: "missing previous port", firstAttempt: true, previousPort: 0, want: true},
		{name: "available previous port", firstAttempt: true, previousPort: availablePort, want: false},
		{name: "occupied previous port", firstAttempt: true, previousPort: busyPort, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAllocateRestartSSHPort(tt.firstAttempt, tt.previousPort); got != tt.want {
				t.Fatalf("shouldAllocateRestartSSHPort(%v, %d) = %v, want %v",
					tt.firstAttempt, tt.previousPort, got, tt.want)
			}
		})
	}
}
