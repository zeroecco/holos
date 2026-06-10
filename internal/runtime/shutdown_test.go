package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/config"
)

func TestShutdownTimingConstants(t *testing.T) {
	t.Parallel()

	if qmpHandshakeTimeout <= 0 {
		t.Fatalf("qmpHandshakeTimeout = %s, want positive", qmpHandshakeTimeout)
	}
	if sigtermGrace <= qmpHandshakeTimeout {
		t.Fatalf("sigtermGrace = %s, want greater than qmpHandshakeTimeout %s", sigtermGrace, qmpHandshakeTimeout)
	}
	if waitForExitPollInterval <= 0 || waitForExitPollInterval > time.Second {
		t.Fatalf("waitForExitPollInterval = %s, want >0 and <=1s", waitForExitPollInterval)
	}
}

func TestStopGraceDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sec  int
		want time.Duration
	}{
		{name: "configured", sec: 7, want: 7 * time.Second},
		{name: "zero default", sec: 0, want: time.Duration(config.DefaultStopGracePeriodSec) * time.Second},
		{name: "negative default", sec: -1, want: time.Duration(config.DefaultStopGracePeriodSec) * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stopGraceDuration(tt.sec); got != tt.want {
				t.Fatalf("stopGraceDuration(%d) = %s, want %s", tt.sec, got, tt.want)
			}
		})
	}
}

func TestQMPPowerdownStopsInstanceWithoutQMPPath(t *testing.T) {
	t.Parallel()

	if qmpPowerdownStopsInstance(InstanceRecord{PID: 0}) {
		t.Fatal("qmpPowerdownStopsInstance without QMP path = true, want false")
	}
}

func TestStopAllInstancesMarksRecordsStopped(t *testing.T) {
	t.Parallel()

	const (
		firstInstanceIndex  = 0
		secondInstanceIndex = 1
	)

	manager := &Manager{}
	firstInstanceName, _ := testInstanceWorkDir(firstInstanceIndex)
	secondInstanceName, _ := testInstanceWorkDir(secondInstanceIndex)
	instances := []InstanceRecord{
		{Name: firstInstanceName, PID: 0, Status: InstanceStatusRunning},
		{Name: secondInstanceName, PID: 0, Status: InstanceStatusRunning},
	}

	before := time.Now().UTC()
	manager.stopAllInstances(instances)
	after := time.Now().UTC()

	for _, inst := range instances {
		if inst.Status != InstanceStatusStopped {
			t.Fatalf("%s status = %q, want %q", inst.Name, inst.Status, InstanceStatusStopped)
		}
		if inst.PID != 0 {
			t.Fatalf("%s PID = %d, want 0", inst.Name, inst.PID)
		}
		assertTimeBetween(t, inst.Name+" LastExitTime", inst.LastExitTime, before, after)
	}
}

func TestMarkInstanceStopped(t *testing.T) {
	t.Parallel()

	inst := InstanceRecord{PID: 123, Status: InstanceStatusRunning}
	before := time.Now().UTC()
	markInstanceStopped(&inst)
	after := time.Now().UTC()

	if inst.Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", inst.Status, InstanceStatusStopped)
	}
	if inst.PID != 0 {
		t.Fatalf("PID = %d, want 0", inst.PID)
	}
	assertTimeBetween(t, "LastExitTime", inst.LastExitTime, before, after)
}

func TestRemoveInstancesStopsAndRemovesWorkDirs(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	workDir := filepath.Join(t.TempDir(), "instance")
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	instances := []InstanceRecord{
		{Name: testInstanceName(testPathService, testPathIndex), PID: 0, Status: InstanceStatusRunning, WorkDir: workDir},
	}

	before := time.Now().UTC()
	manager.removeInstances(instances)
	after := time.Now().UTC()

	if instances[0].Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", instances[0].Status, InstanceStatusStopped)
	}
	assertTimeBetween(t, "LastExitTime", instances[0].LastExitTime, before, after)
	if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed workdir stat err = %v, want os.ErrNotExist", err)
	}
}

func assertTimeBetween(t *testing.T, name string, got, before, after time.Time) {
	t.Helper()

	if got.Before(before) || got.After(after) {
		t.Fatalf("%s = %s, want between %s and %s", name, got, before, after)
	}
}

func TestRemoveInstanceDir(t *testing.T) {
	t.Parallel()

	workDir := filepath.Join(t.TempDir(), "instance")
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	logPath := filepath.Join(workDir, instanceConsoleLogFilename)
	if err := os.WriteFile(logPath, []byte("boot"), qemuLogPerm); err != nil {
		t.Fatalf("write workdir file: %v", err)
	}

	removeInstanceDir(InstanceRecord{WorkDir: workDir})
	if _, err := os.Stat(workDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed workdir stat err = %v, want os.ErrNotExist", err)
	}
}

func TestRemoveInstanceDirAllowsEmptyWorkDir(t *testing.T) {
	t.Parallel()

	removeInstanceDir(InstanceRecord{})
}
