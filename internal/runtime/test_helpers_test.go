package runtime

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/qemu"
)

func assertErrorContains(t *testing.T, err error, wants ...string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wants)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want substring %q", err, want)
		}
	}
}

func assertStringContains(t *testing.T, got, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertIntSliceEqual(t *testing.T, name string, got, want []int) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertLaunchSpecIdentity(t *testing.T, spec qemu.LaunchSpec, name string, index int) {
	t.Helper()

	if spec.Name != name || spec.Index != index {
		t.Fatalf("identity = (%q,%d), want (%q,%d)", spec.Name, spec.Index, name, index)
	}
}

func assertLaunchSpecPaths(t *testing.T, spec qemu.LaunchSpec, overlayPath, seedPath, logPath, serialPath, qmpPath string) {
	t.Helper()

	if spec.OverlayPath != overlayPath || spec.SeedPath != seedPath || spec.LogPath != logPath {
		t.Fatalf("disk/log paths = %+v, want overlay=%q seed=%q log=%q", spec, overlayPath, seedPath, logPath)
	}
	if spec.SerialPath != serialPath || spec.QMPPath != qmpPath {
		t.Fatalf("control paths = %+v, want serial=%q qmp=%q", spec, serialPath, qmpPath)
	}
}

func assertLaunchSpecVolumes(t *testing.T, spec qemu.LaunchSpec, want []qemu.VolumeAttachment) {
	t.Helper()

	if !slices.Equal(spec.Volumes, want) {
		t.Fatalf("Volumes = %+v, want %+v", spec.Volumes, want)
	}
}

func assertInstanceRecordIdentity(t *testing.T, record InstanceRecord, name string, index int, workDir string) {
	t.Helper()

	if record.Name != name || record.Index != index || record.WorkDir != workDir {
		t.Fatalf("identity/workdir = %+v, want name=%q index=%d workdir=%q", record, name, index, workDir)
	}
}

func assertInstanceRecordRunning(t *testing.T, record InstanceRecord, pid int) {
	t.Helper()

	if record.PID != pid || record.Status != InstanceStatusRunning {
		t.Fatalf("process state = pid %d status %q, want running pid %d", record.PID, record.Status, pid)
	}
}

func assertInstanceRecordPaths(t *testing.T, record InstanceRecord, overlayPath, seedPath, logPath, serialPath, qmpPath string) {
	t.Helper()

	if record.OverlayPath != overlayPath || record.SeedPath != seedPath || record.LogPath != logPath {
		t.Fatalf("disk/log paths = %+v, want overlay=%q seed=%q log=%q", record, overlayPath, seedPath, logPath)
	}
	if record.SerialPath != serialPath || record.QMPPath != qmpPath {
		t.Fatalf("control paths = %+v, want serial=%q qmp=%q", record, serialPath, qmpPath)
	}
}

func assertInstanceRecordPorts(t *testing.T, record InstanceRecord, ports []qemu.PortMapping, sshPort int) {
	t.Helper()

	if !slices.Equal(record.Ports, ports) || record.SSHPort != sshPort {
		t.Fatalf("ports = %+v ssh=%d, want ports=%+v ssh=%d", record.Ports, record.SSHPort, ports, sshPort)
	}
}

func assertInstanceRecordTiming(t *testing.T, record InstanceRecord, stopGracePeriodSec int, startedAt time.Time) {
	t.Helper()

	if record.StopGracePeriodSec != stopGracePeriodSec || !record.LastStarted.Equal(startedAt) {
		t.Fatalf("timing = grace %d started %s, want grace %d started %s",
			record.StopGracePeriodSec, record.LastStarted, stopGracePeriodSec, startedAt)
	}
}
