package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
)

const (
	testVolumeDeclaredSize  = int64(20 << 20)
	testVolumeOrphanProject = "orphan"
)

func TestVolumeRecordsForProject(t *testing.T) {
	t.Parallel()

	records := volumeRecordsForProject(&compose.Project{
		Volumes: map[string]compose.VolumeSpec{
			testCacheVolumeName: {Name: testCacheVolumeName, SizeBytes: 5 << 20},
			testVolumeName:      {Name: testVolumeName, SizeBytes: testVolumeDeclaredSize},
		},
	})

	if len(records) != 2 {
		t.Fatalf("volume records = %+v, want two", records)
	}
	if records[0].Name != testCacheVolumeName || records[1].Name != testVolumeName {
		t.Fatalf("volume records not sorted by name: %+v", records)
	}
}

func TestListVolumesCombinesBackingFilesRecordsAndAttachments(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)

	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	orphanBacking := writeTestProjectVolumeBacking(t, stateDir, testVolumeOrphanProject, testCacheVolumeName, "orphan")

	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	if err := os.Symlink(backing, volumeLinkPath(workDir, testVolumeName)); err != nil {
		t.Fatalf("symlink volume: %v", err)
	}
	if err := os.WriteFile(volumeLinkPath(workDir, testCacheVolumeName), []byte("not a symlink"), 0o600); err != nil {
		t.Fatalf("write regular volume-shaped file: %v", err)
	}

	record := &ProjectRecord{
		Name: testVolumeProject,
		Volumes: []VolumeRecord{
			{Name: testVolumeName, SizeBytes: testVolumeDeclaredSize},
		},
		Services: []ServiceRecord{
			{
				Name: testVolumeService,
				Instances: []InstanceRecord{
					{
						Name:    instanceDirName(testVolumeService, 0),
						Status:  InstanceStatusStopped,
						WorkDir: workDir,
					},
				},
			},
		},
	}
	if err := manager.saveProject(record); err != nil {
		t.Fatalf("save project: %v", err)
	}

	volumes, err := manager.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("volumes = %+v, want two", volumes)
	}

	assertVolumeInfo(t, volumes[0], VolumeInfo{
		Project:   testVolumeProject,
		Name:      testVolumeName,
		SizeBytes: testVolumeDeclaredSize,
		Path:      backing,
		Attachments: []VolumeAttachmentInfo{
			{Service: testVolumeService, Instance: instanceDirName(testVolumeService, 0), Status: InstanceStatusStopped},
		},
	})
	assertVolumeInfo(t, volumes[1], VolumeInfo{
		Project:   testVolumeOrphanProject,
		Name:      testCacheVolumeName,
		SizeBytes: int64(len("orphan")),
		Path:      orphanBacking,
	})
}

func TestRemoveVolumeDeletesDetachedBackingFile(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)

	if err := manager.RemoveVolume(testVolumeProject, testVolumeName); err != nil {
		t.Fatalf("RemoveVolume: %v", err)
	}
	if _, err := os.Stat(backing); !os.IsNotExist(err) {
		t.Fatalf("removed backing stat err = %v, want not exist", err)
	}
}

func TestRemoveVolumeRefusesAttachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	if err := os.Symlink(backing, volumeLinkPath(workDir, testVolumeName)); err != nil {
		t.Fatalf("symlink volume: %v", err)
	}
	if err := manager.saveProject(&ProjectRecord{
		Name: testVolumeProject,
		Services: []ServiceRecord{
			{
				Name: testVolumeService,
				Instances: []InstanceRecord{
					{Name: instanceDirName(testVolumeService, 0), Status: InstanceStatusRunning, WorkDir: workDir},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	err := manager.RemoveVolume(testVolumeProject, testVolumeName)
	assertErrorContains(t, err, "attached", instanceDirName(testVolumeService, 0))
	if _, statErr := os.Stat(backing); statErr != nil {
		t.Fatalf("backing stat after refused remove: %v", statErr)
	}
}

func TestRemoveVolumeReportsMissingVolume(t *testing.T) {
	t.Parallel()

	err := NewManager(t.TempDir()).RemoveVolume(testVolumeProject, testMissingVolumeName)
	assertErrorContains(t, err, "not found")
}

func TestRemoveVolumeRejectsInvalidName(t *testing.T) {
	t.Parallel()

	err := NewManager(t.TempDir()).RemoveVolume(testVolumeProject, "../data")
	assertErrorContains(t, err, "invalid volume name")
}

func TestExportVolumeCopiesDetachedBackingFile(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	writeTestVolumeBacking(t, stateDir, testVolumeName)
	destination := filepath.Join(t.TempDir(), "data-export.qcow2")

	if err := manager.ExportVolume(testVolumeProject, testVolumeName, destination); err != nil {
		t.Fatalf("ExportVolume: %v", err)
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(payload) != testVolumePayload {
		t.Fatalf("export payload = %q, want %q", string(payload), testVolumePayload)
	}
}

func TestExportVolumeRefusesExistingDestination(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	writeTestVolumeBacking(t, stateDir, testVolumeName)
	destination := filepath.Join(t.TempDir(), "data-export.qcow2")
	if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed export destination: %v", err)
	}

	err := manager.ExportVolume(testVolumeProject, testVolumeName, destination)
	assertErrorContains(t, err, "already exists")
	payload, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatalf("read destination: %v", readErr)
	}
	if string(payload) != "existing" {
		t.Fatalf("destination payload = %q, want existing", string(payload))
	}
}

func TestExportVolumeRefusesAttachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	if err := os.Symlink(backing, volumeLinkPath(workDir, testVolumeName)); err != nil {
		t.Fatalf("symlink volume: %v", err)
	}
	if err := manager.saveProject(&ProjectRecord{
		Name: testVolumeProject,
		Services: []ServiceRecord{
			{
				Name: testVolumeService,
				Instances: []InstanceRecord{
					{Name: instanceDirName(testVolumeService, 0), Status: InstanceStatusRunning, WorkDir: workDir},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "data-export.qcow2")
	err := manager.ExportVolume(testVolumeProject, testVolumeName, destination)
	assertErrorContains(t, err, "attached", instanceDirName(testVolumeService, 0))
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination stat err = %v, want not exist", statErr)
	}
}

func TestSnapshotVolumeCreatesInternalSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	logPath := installQEMUImgVolumeMock(t)

	if err := manager.SnapshotVolume(testVolumeProject, testVolumeName, "before-upgrade"); err != nil {
		t.Fatalf("SnapshotVolume: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"snapshot", "-c", "before-upgrade", backing}
	assertStringSliceEqual(t, "qemu-img args", args, want)
}

func TestSnapshotVolumeRefusesAttachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	if err := os.Symlink(backing, volumeLinkPath(workDir, testVolumeName)); err != nil {
		t.Fatalf("symlink volume: %v", err)
	}
	if err := manager.saveProject(&ProjectRecord{
		Name: testVolumeProject,
		Services: []ServiceRecord{
			{
				Name: testVolumeService,
				Instances: []InstanceRecord{
					{Name: instanceDirName(testVolumeService, 0), Status: InstanceStatusRunning, WorkDir: workDir},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	err := manager.SnapshotVolume(testVolumeProject, testVolumeName, "before-upgrade")
	assertErrorContains(t, err, "attached", instanceDirName(testVolumeService, 0))
}

func TestSnapshotVolumeRejectsInvalidSnapshotName(t *testing.T) {
	t.Parallel()

	err := NewManager(t.TempDir()).SnapshotVolume(testVolumeProject, testVolumeName, "../bad")
	assertErrorContains(t, err, "invalid snapshot name")
}

func TestRestoreVolumeSnapshotAppliesSnapshot(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	logPath := installQEMUImgVolumeMock(t)
	if err := manager.RestoreVolumeSnapshot(testVolumeProject, testVolumeName, "before-upgrade"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	assertStringSliceEqual(t, "qemu-img args", strings.Split(strings.TrimSpace(string(data)), "\n"), []string{"snapshot", "-a", "before-upgrade", backing})
}

func TestExportVolumeSnapshotUsesSnapshotInput(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	logPath := installQEMUImgVolumeMock(t)
	destination := filepath.Join(t.TempDir(), "snapshot.qcow2")
	if err := manager.ExportVolumeSnapshot(testVolumeProject, testVolumeName, "before-upgrade", destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	assertStringSliceEqual(t, "qemu-img args", strings.Split(strings.TrimSpace(string(data)), "\n"), []string{"convert", "-O", "qcow2", "-l", "before-upgrade", backing, destination})
}

func TestResizeVolumeResizesDetachedVolumeAndUpdatesRecord(t *testing.T) {
	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	if err := manager.saveProject(&ProjectRecord{
		Name:    testVolumeProject,
		Volumes: []VolumeRecord{{Name: testVolumeName, SizeBytes: testVolumeDeclaredSize}},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}
	logPath := installQEMUImgVolumeMock(t)

	const newSize = int64(30 << 20)
	if err := manager.ResizeVolume(testVolumeProject, testVolumeName, newSize, false); err != nil {
		t.Fatalf("ResizeVolume: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read qemu-img log: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"resize", backing, "31457280"}
	assertStringSliceEqual(t, "qemu-img args", args, want)

	record, err := manager.loadProject(testVolumeProject)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if len(record.Volumes) != 1 || record.Volumes[0].Name != testVolumeName || record.Volumes[0].SizeBytes != newSize {
		t.Fatalf("record volumes = %+v, want resized %s", record.Volumes, testVolumeName)
	}
}

func TestResizeVolumeRefusesAttachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := NewManager(stateDir)
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)
	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	if err := os.Symlink(backing, volumeLinkPath(workDir, testVolumeName)); err != nil {
		t.Fatalf("symlink volume: %v", err)
	}
	if err := manager.saveProject(&ProjectRecord{
		Name: testVolumeProject,
		Services: []ServiceRecord{
			{
				Name: testVolumeService,
				Instances: []InstanceRecord{
					{Name: instanceDirName(testVolumeService, 0), Status: InstanceStatusRunning, WorkDir: workDir},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	err := manager.ResizeVolume(testVolumeProject, testVolumeName, 30<<20, false)
	assertErrorContains(t, err, "attached", instanceDirName(testVolumeService, 0))
}

func TestResizeVolumeRejectsInvalidSize(t *testing.T) {
	t.Parallel()

	err := NewManager(t.TempDir()).ResizeVolume(testVolumeProject, testVolumeName, 0, false)
	assertErrorContains(t, err, "positive")
}

func TestVolumeResizeArgsAllowsExplicitShrink(t *testing.T) {
	t.Parallel()

	got := volumeResizeArgs("/volumes/data.qcow2", 10<<20, true)
	want := []string{"resize", "--shrink", "/volumes/data.qcow2", "10485760"}
	assertStringSliceEqual(t, "resize args", got, want)
}

func TestVolumeNameFromLink(t *testing.T) {
	t.Parallel()

	got, ok := volumeNameFromLink("vol-data.qcow2")
	if !ok || got != testVolumeName {
		t.Fatalf("volumeNameFromLink = %q, %v; want %q, true", got, ok, testVolumeName)
	}
	for _, name := range []string{"data.qcow2", "vol-.qcow2", "vol-data.img"} {
		if got, ok := volumeNameFromLink(name); ok {
			t.Fatalf("volumeNameFromLink(%q) = %q, true; want false", name, got)
		}
	}
}

func writeTestProjectVolumeBacking(t *testing.T, stateDir, project, name, content string) string {
	t.Helper()

	root := volumesRoot(stateDir, project)
	if err := os.MkdirAll(root, stateDirPerm); err != nil {
		t.Fatalf("create volumes root: %v", err)
	}
	backing := volumeBackingPath(stateDir, project, name)
	if err := os.WriteFile(backing, []byte(content), 0o600); err != nil {
		t.Fatalf("write backing: %v", err)
	}
	return backing
}

func installQEMUImgVolumeMock(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	qemuImg := filepath.Join(dir, qemuImgDefault)
	logPath := filepath.Join(dir, qemuImgDefault+".log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$" + testQEMUImgLogEnv + "\"\n"
	if err := os.WriteFile(qemuImg, []byte(script), 0o755); err != nil {
		t.Fatalf("write qemu-img mock: %v", err)
	}
	t.Setenv(qemuImgEnv, qemuImg)
	t.Setenv(testQEMUImgLogEnv, logPath)
	return logPath
}

func assertVolumeInfo(t *testing.T, got, want VolumeInfo) {
	t.Helper()

	if got.Project != want.Project ||
		got.Name != want.Name ||
		got.SizeBytes != want.SizeBytes ||
		filepath.Clean(got.Path) != filepath.Clean(want.Path) {
		t.Fatalf("volume info = %+v, want %+v", got, want)
	}
	if len(got.Attachments) != len(want.Attachments) {
		t.Fatalf("attachments = %+v, want %+v", got.Attachments, want.Attachments)
	}
	for i := range want.Attachments {
		if got.Attachments[i] != want.Attachments[i] {
			t.Fatalf("attachment[%d] = %+v, want %+v", i, got.Attachments[i], want.Attachments[i])
		}
	}
}
