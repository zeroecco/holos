package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotInstanceRootCreatesStoppedOverlaySnapshot(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	overlayPath := writeTestInstanceOverlay(t, stateDir, testPathProject, testPathService, 0)
	if err := manager.saveProject(&ProjectRecord{
		Name: testPathProject,
		Services: []ServiceRecord{{
			Name: testPathService,
			Instances: []InstanceRecord{{
				Name:        instanceDirName(testPathService, 0),
				Index:       0,
				Status:      InstanceStatusStopped,
				OverlayPath: overlayPath,
			}},
		}},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	logPath := installQEMUImgVolumeMock(t)

	if err := manager.SnapshotInstanceRoot(testPathProject, instanceDirName(testPathService, 0), "before-upgrade"); err != nil {
		t.Fatalf("SnapshotInstanceRoot: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"snapshot", "-c", "before-upgrade", overlayPath}
	assertStringSliceEqual(t, "qemu-img args", args, want)
}

func TestSnapshotInstanceRootRefusesRunningInstance(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	overlayPath := writeTestInstanceOverlay(t, stateDir, testPathProject, testPathService, 0)
	pid, stop := startTestQEMUProcess(t)
	defer stop()
	if err := manager.saveProject(&ProjectRecord{
		Name: testPathProject,
		Services: []ServiceRecord{{
			Name: testPathService,
			Instances: []InstanceRecord{{
				Name:        instanceDirName(testPathService, 0),
				Index:       0,
				PID:         pid,
				Status:      InstanceStatusRunning,
				OverlayPath: overlayPath,
			}},
		}},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	err := manager.SnapshotInstanceRoot(testPathProject, instanceDirName(testPathService, 0), "before-upgrade")
	assertErrorContains(t, err, "running", "stop it before snapshotting")
}

func TestSnapshotInstanceRootRejectsInvalidSnapshotName(t *testing.T) {
	t.Parallel()

	err := NewManager(t.TempDir()).SnapshotInstanceRoot(testPathProject, instanceDirName(testPathService, 0), "../bad")
	assertErrorContains(t, err, "invalid snapshot name")
}

func TestParseSnapshotList(t *testing.T) {
	output := `Snapshot list:
ID        TAG                     VM SIZE                DATE        VM CLOCK
1         before-upgrade          0 B     2026-07-09 10:00:00   00:00:00.000
2         clean                   0 B     2026-07-09 10:01:00   00:00:00.000
`
	got := parseSnapshotList(output)
	if len(got) != 2 || got[0].Name != "before-upgrade" || got[1].Name != "clean" {
		t.Fatalf("parseSnapshotList = %+v", got)
	}
}

func TestRestoreInstanceSnapshotAppliesSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	overlayPath := writeTestInstanceOverlay(t, stateDir, testPathProject, testPathService, 0)
	if err := manager.saveProject(&ProjectRecord{Name: testPathProject, Services: []ServiceRecord{{Name: testPathService, Instances: []InstanceRecord{{Name: instanceDirName(testPathService, 0), Status: InstanceStatusStopped, OverlayPath: overlayPath}}}}}); err != nil {
		t.Fatal(err)
	}
	logPath := installQEMUImgVolumeMock(t)
	if err := manager.RestoreInstanceSnapshot(testPathProject, instanceDirName(testPathService, 0), "before-upgrade"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	assertStringSliceEqual(t, "qemu-img args", strings.Split(strings.TrimSpace(string(data)), "\n"), []string{"snapshot", "-a", "before-upgrade", overlayPath})
}

func TestExportInstanceSnapshotUsesSnapshotInput(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	overlayPath := writeTestInstanceOverlay(t, stateDir, testPathProject, testPathService, 0)
	if err := manager.saveProject(&ProjectRecord{Name: testPathProject, Services: []ServiceRecord{{Name: testPathService, Instances: []InstanceRecord{{Name: instanceDirName(testPathService, 0), Status: InstanceStatusStopped, OverlayPath: overlayPath}}}}}); err != nil {
		t.Fatal(err)
	}
	logPath := installQEMUImgVolumeMock(t)
	destination := filepath.Join(t.TempDir(), "snapshot.qcow2")
	if err := manager.ExportInstanceSnapshot(testPathProject, instanceDirName(testPathService, 0), "before-upgrade", destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	assertStringSliceEqual(t, "qemu-img args", strings.Split(strings.TrimSpace(string(data)), "\n"), []string{"convert", "-O", "qcow2", "-l", "before-upgrade", overlayPath, destination})
}

func writeTestInstanceOverlay(t *testing.T, stateDir, project, service string, index int) string {
	t.Helper()

	workDir := projectInstanceDir(stateDir, project, service, index)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	overlayPath := filepath.Join(workDir, instanceOverlayFilename)
	if err := os.WriteFile(overlayPath, []byte("overlay"), 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	return overlayPath
}

func startTestQEMUProcess(t *testing.T) (int, func()) {
	t.Helper()

	dir := t.TempDir()
	qemuPath := filepath.Join(dir, "qemu-system-test")
	if err := os.Symlink("/bin/sleep", qemuPath); err != nil {
		t.Fatalf("symlink qemu process: %v", err)
	}
	cmd := exec.Command(qemuPath, "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start qemu process: %v", err)
	}
	return cmd.Process.Pid, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}
