package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	testVolumeProject     = "demo"
	testVolumeService     = "web"
	testVolumeName        = "data"
	testCacheVolumeName   = "cache"
	testMissingVolumeName = "missing"
	testVolumePayload     = "volume"
)

func testVolumeWorkDir(stateDir string) string {
	return projectInstanceDir(stateDir, testVolumeProject, testVolumeService, 0)
}

func writeTestVolumeBacking(t *testing.T, stateDir, name string) string {
	t.Helper()

	root := volumesRoot(stateDir, testVolumeProject)
	if err := os.MkdirAll(root, stateDirPerm); err != nil {
		t.Fatalf("create volumes root: %v", err)
	}
	backing := volumeBackingPath(stateDir, testVolumeProject, name)
	if err := os.WriteFile(backing, []byte(testVolumePayload), 0o600); err != nil {
		t.Fatalf("write backing: %v", err)
	}
	return backing
}

type testVolumeAttachmentWant struct {
	name     string
	diskPath string
	readOnly bool
}

func assertVolumeAttachment(t *testing.T, got qemu.VolumeAttachment, want testVolumeAttachmentWant) {
	t.Helper()

	if got.Name != want.name || got.DiskPath != want.diskPath || got.ReadOnly != want.readOnly {
		t.Fatalf("attachment = %+v, want %+v", got, want)
	}
}

func TestVolumePaths(t *testing.T) {
	stateDir := filepath.FromSlash("state/holos")
	workDir := testVolumeWorkDir(stateDir)

	if got, want := volumesRoot(stateDir, testVolumeProject), filepath.FromSlash("state/holos/volumes/demo"); got != want {
		t.Fatalf("volumesRoot = %q, want %q", got, want)
	}
	if got, want := volumeBackingPath(stateDir, testVolumeProject, testVolumeName), filepath.FromSlash("state/holos/volumes/demo/data.qcow2"); got != want {
		t.Fatalf("volumeBackingPath = %q, want %q", got, want)
	}

	if got, want := volumeLinkName(testVolumeName), "vol-data.qcow2"; got != want {
		t.Fatalf("volumeLinkName = %q, want %q", got, want)
	}
	if got, want := volumeLinkPath(workDir, testVolumeName), filepath.FromSlash("state/holos/instances/demo/web-0/vol-data.qcow2"); got != want {
		t.Fatalf("volumeLinkPath = %q, want %q", got, want)
	}
}

func TestVolumeCreateArgs(t *testing.T) {
	t.Parallel()

	path := filepath.FromSlash("/state/volumes/demo/data.qcow2")
	got := volumeCreateArgs(path, 10<<20)
	want := []string{
		"create",
		"-f", "qcow2",
		path,
		"10485760",
	}
	assertStringSliceEqual(t, "volumeCreateArgs", got, want)
}

func TestVolumeAttachmentForMount(t *testing.T) {
	t.Parallel()

	workDir := filepath.Join("/state/instances", testVolumeProject, instanceDirName(testVolumeService, 0))
	diskPath := volumeLinkPath(workDir, testVolumeName)
	attachment := volumeAttachmentForMount(config.Mount{
		VolumeName: testVolumeName,
		ReadOnly:   true,
	}, diskPath)
	assertVolumeAttachment(t, attachment, testVolumeAttachmentWant{
		name:     testVolumeName,
		diskPath: diskPath,
		readOnly: true,
	})
}

func TestMaterializeInstanceVolumesCreatesLinks(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	workDir := testVolumeWorkDir(stateDir)
	if err := os.MkdirAll(workDir, stateDirPerm); err != nil {
		t.Fatalf("create workdir: %v", err)
	}
	backing := writeTestVolumeBacking(t, stateDir, testVolumeName)

	attachments, err := materializeInstanceVolumes(stateDir, testVolumeProject, workDir, []config.Mount{
		{Kind: config.MountKindBind},
		{Kind: config.MountKindVolume, VolumeName: testVolumeName, ReadOnly: true},
	})
	if err != nil {
		t.Fatalf("materializeInstanceVolumes: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want one volume attachment", attachments)
	}
	link := volumeLinkPath(workDir, testVolumeName)
	assertVolumeAttachment(t, attachments[0], testVolumeAttachmentWant{
		name:     testVolumeName,
		diskPath: link,
		readOnly: true,
	})
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != backing {
		t.Fatalf("volume link target = %q, want %q", target, backing)
	}
}

func TestMaterializeInstanceVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	workDir := t.TempDir()

	attachment, ok, err := materializeInstanceVolume(stateDir, testVolumeProject, workDir, config.Mount{Kind: config.MountKindBind})
	if err != nil {
		t.Fatalf("materializeInstanceVolume bind: %v", err)
	}
	if ok || attachment != (qemu.VolumeAttachment{}) {
		t.Fatalf("materializeInstanceVolume bind = (%+v, %v), want zero,false", attachment, ok)
	}

	writeTestVolumeBacking(t, stateDir, testVolumeName)

	attachment, ok, err = materializeInstanceVolume(stateDir, testVolumeProject, workDir, config.Mount{
		Kind:       config.MountKindVolume,
		VolumeName: testVolumeName,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("materializeInstanceVolume volume: %v", err)
	}
	if !ok {
		t.Fatal("materializeInstanceVolume volume ok = false, want true")
	}
	link := volumeLinkPath(workDir, testVolumeName)
	assertVolumeAttachment(t, attachment, testVolumeAttachmentWant{
		name:     testVolumeName,
		diskPath: link,
		readOnly: true,
	})
}

func TestMaterializeInstanceVolumesReplacesStaleLink(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	workDir := t.TempDir()
	backing := writeTestVolumeBacking(t, stateDir, testCacheVolumeName)
	link := volumeLinkPath(workDir, testCacheVolumeName)
	if err := os.Symlink("/stale/target", link); err != nil {
		t.Fatalf("seed stale link: %v", err)
	}

	if _, err := materializeInstanceVolumes(stateDir, testVolumeProject, workDir, []config.Mount{
		{Kind: config.MountKindVolume, VolumeName: testCacheVolumeName},
	}); err != nil {
		t.Fatalf("materializeInstanceVolumes: %v", err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != backing {
		t.Fatalf("volume link target = %q, want %q", target, backing)
	}
}

func TestMaterializeInstanceVolumesRequiresBackingFile(t *testing.T) {
	t.Parallel()

	_, err := materializeInstanceVolumes(t.TempDir(), testVolumeProject, t.TempDir(), []config.Mount{
		{Kind: config.MountKindVolume, VolumeName: testMissingVolumeName},
	})
	if err == nil {
		t.Fatal("materializeInstanceVolumes succeeded with missing backing file")
	}
	assertErrorContains(t, err, `volume "`+testMissingVolumeName+`" backing`)
}
