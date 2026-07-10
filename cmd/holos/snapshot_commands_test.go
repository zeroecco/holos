package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestRunSnapshotCreateCreatesStoppedRootOverlaySnapshot(t *testing.T) {
	stateDir := t.TempDir()
	workDir := filepath.Join(stateDir, "instances", testSnapshotCommandProject, testSnapshotCommandInstance)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	overlayPath := filepath.Join(workDir, "root.qcow2")
	if err := os.WriteFile(overlayPath, []byte("overlay"), 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	record := testProjectRecord(testSnapshotCommandProject, runtime.ServiceRecord{
		Name: testSnapshotCommandService,
		Instances: []runtime.InstanceRecord{{
			Name:        testSnapshotCommandInstance,
			Index:       0,
			Status:      runtime.InstanceStatusStopped,
			OverlayPath: overlayPath,
		}},
	})
	if err := writeProjectRecord(stateDir, record); err != nil {
		t.Fatalf("write project record: %v", err)
	}
	logPath := installQEMUImgCommandMock(t)

	err := runSnapshotCreate([]string{"--state-dir", stateDir, testSnapshotCommandProject, testSnapshotCommandInstance, "before-upgrade"})
	if err != nil {
		t.Fatalf("runSnapshotCreate: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"snapshot", "-c", "before-upgrade", overlayPath}
	assertStringSliceEqual(t, "qemu-img args", args, want)
}

func TestRunSnapshotsRequiresCreateSubcommand(t *testing.T) {
	t.Parallel()

	err := runSnapshots(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: holos snapshots {create|list|rm}") {
		t.Fatalf("runSnapshots(nil) err = %v, want usage", err)
	}
}

func TestRunSnapshotCreateRequiresProjectInstanceAndSnapshot(t *testing.T) {
	t.Parallel()

	err := runSnapshotCreate(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: holos snapshots create") {
		t.Fatalf("runSnapshotCreate(nil) err = %v, want usage", err)
	}
}

const (
	testSnapshotCommandProject  = "demo"
	testSnapshotCommandService  = "web"
	testSnapshotCommandInstance = "web-0"
)
