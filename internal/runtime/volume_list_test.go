package runtime

import (
	"os"
	"path/filepath"
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
						PID:     os.Getpid(),
						Status:  InstanceStatusRunning,
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
			{Service: testVolumeService, Instance: instanceDirName(testVolumeService, 0), Status: InstanceStatusRunning},
		},
	})
	assertVolumeInfo(t, volumes[1], VolumeInfo{
		Project:   testVolumeOrphanProject,
		Name:      testCacheVolumeName,
		SizeBytes: int64(len("orphan")),
		Path:      orphanBacking,
	})
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
