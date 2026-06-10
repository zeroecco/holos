package cloudinit

import (
	"slices"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestSerialConsoleFilesSystemd(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{CloudInit: config.CloudInit{User: "debian"}}
	files := serialConsoleFiles(manifest, familySystemd)
	wantAutologin := serialConsoleAgettyContent("debian")
	want := []ccFile{
		{
			Path:        serialConsoleGrubPath,
			Content:     serialConsoleGrubContent,
			Permissions: config.DefaultFilePermissions,
			Owner:       config.DefaultFileOwner,
		},
		{
			Path:        serialConsoleAutologinPath,
			Content:     wantAutologin,
			Permissions: config.DefaultFilePermissions,
			Owner:       config.DefaultFileOwner,
		},
	}
	if !slices.Equal(files, want) {
		t.Fatalf("serialConsoleFiles = %+v, want %+v", files, want)
	}
}

func TestSerialConsoleAgettyContent(t *testing.T) {
	t.Parallel()

	got := serialConsoleAgettyContent("debian")
	want := "[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin debian --noclear %I $TERM\n"
	if got != want {
		t.Fatalf("serialConsoleAgettyContent = %q, want %q", got, want)
	}
}

func TestSerialConsoleSkipsOpenRC(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{CloudInit: config.CloudInit{User: "alpine"}}
	if files := serialConsoleFiles(manifest, familyOpenRC); len(files) != 0 {
		t.Fatalf("serialConsoleFiles openrc = %+v, want empty", files)
	}
	if cmds := serialConsoleRunCmd(familyOpenRC); len(cmds) != 0 {
		t.Fatalf("serialConsoleRunCmd openrc = %+v, want empty", cmds)
	}
}

func TestSerialConsoleRunCmdSystemd(t *testing.T) {
	t.Parallel()

	cmds := serialConsoleRunCmd(familySystemd)
	want := []string{serialGettySystemdCmd}
	assertStringSliceEqual(t, "serialConsoleRunCmd", cmds, want)
}
