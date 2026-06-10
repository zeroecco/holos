package cloudinit

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestVolumeMountRunCmdUsesCommandSeparator(t *testing.T) {
	t.Parallel()

	cmds := volumeMountRunCmd(config.Manifest{Mounts: []config.Mount{
		{Kind: config.MountKindVolume, VolumeName: "data", Target: "/var/lib/data"},
	}})
	assertStringSliceLen(t, "volumeMountRunCmd", cmds, 1)
	assertContains(t, cmds[0], volumeCommandSeparator)
}

func TestVolumeMountSteps(t *testing.T) {
	t.Parallel()

	mount := config.Mount{Kind: config.MountKindVolume, VolumeName: "data", Target: "/var/lib/data"}
	dev := volumeDevicePathPrefix + volumeLabel(mount.VolumeName)
	got := volumeMountSteps(mount)
	want := []string{
		volumeSettleCommand(dev),
		volumeMkfsGuardCommand(dev, volumeLabel(mount.VolumeName)),
		volumeMkdirCommand(mount.Target),
		volumeFstabAppendCommand(dev, mount.Target, volumeFstabDefaultOpts),
		volumeMountCommand(mount.VolumeName, mount.Target),
	}
	assertStringSliceEqual(t, "volumeMountSteps", got, want)
}

func TestVolumeMountStepsReadOnlyOmitsMkfs(t *testing.T) {
	t.Parallel()

	mount := config.Mount{Kind: config.MountKindVolume, VolumeName: "shared", Target: "/srv/shared", ReadOnly: true}
	got := volumeMountSteps(mount)
	for _, step := range got {
		assertOmits(t, step, volumeMkfsCommand)
	}
	assertContains(t, got[len(got)-2], volumeFstabReadOnlyOpts)
}

func TestVolumeShellCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "settle",
			got:  volumeSettleCommand("/dev/disk/by-id/virtio-vol-data"),
			want: "udevadm settle --exit-if-exists='/dev/disk/by-id/virtio-vol-data' || true",
		},
		{
			name: "mkfs guard",
			got:  volumeMkfsGuardCommand("/dev/disk/by-id/virtio-vol-data", "vol-data"),
			want: "if [ -b '/dev/disk/by-id/virtio-vol-data' ] && ! blkid '/dev/disk/by-id/virtio-vol-data' >/dev/null 2>&1; then mkfs.ext4 -F -L 'vol-data' '/dev/disk/by-id/virtio-vol-data'; fi",
		},
		{
			name: "mkdir",
			got:  volumeMkdirCommand("/srv/my data"),
			want: "mkdir -p '/srv/my data'",
		},
		{
			name: "fstab append",
			got:  volumeFstabAppendCommand("/dev/disk/by-id/virtio-vol-data", "/srv/my data", volumeFstabReadOnlyOpts),
			want: "grep -qE ' /srv/my data ' /etc/fstab || echo '/dev/disk/by-id/virtio-vol-data /srv/my data ext4 ro,nofail 0 2' >> /etc/fstab",
		},
		{
			name: "mount",
			got:  volumeMountCommand("data", "/srv/my data"),
			want: "mountpoint -q '/srv/my data' || mount '/srv/my data' || { echo 'holos: failed to mount volume data at /srv/my data' >&2; exit 1; }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("command = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestShquote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain path",
			input: "/srv/data",
			want:  "'/srv/data'",
		},
		{
			name:  "single quote",
			input: "/srv/it'll-work",
			want:  `'/srv/it'\''ll-work'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shquote(tt.input); got != tt.want {
				t.Fatalf("shquote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
