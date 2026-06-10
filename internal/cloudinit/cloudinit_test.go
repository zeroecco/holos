package cloudinit

import (
	"slices"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("expected content to contain %q, got:\n%s", want, got)
		}
	}
}

func assertOmits(t *testing.T, got string, forbidden ...string) {
	t.Helper()

	for _, f := range forbidden {
		if strings.Contains(got, f) {
			t.Fatalf("expected content to omit %q, got:\n%s", f, got)
		}
	}
}

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func assertStringSliceLen(t *testing.T, name string, got []string, wantLen int) {
	t.Helper()

	if len(got) != wantLen {
		t.Fatalf("%s len = %d, want %d: %v", name, len(got), wantLen, got)
	}
}

func TestRenderIncludesUserFilesAndCommands(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name: "api",
		CloudInit: config.CloudInit{
			User:              "ubuntu",
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAATEST holos"},
			Packages:          []string{"curl"},
			BootCmd:           []string{"echo booting"},
			RunCmd:            []string{"systemctl restart api"},
			WriteFiles: []config.WriteFile{
				{
					Path:        "/etc/api.env",
					Content:     "PORT=8080\nMODE=prod\n",
					Permissions: config.DefaultFilePermissions,
					Owner:       config.DefaultFileOwner,
				},
			},
		},
	}

	userData, metaData, _ := Render(manifest, "api-0", 0)

	assertContains(t, userData,
		"#cloud-config",
		"hostname: api-0",
		"name: ubuntu",
		"- curl",
		"path: /etc/api.env",
		"PORT=8080",
		"- systemctl restart api",
	)

	assertContains(t, metaData, "instance-id: api-0")
}

func TestRenderMetaData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		manifest     config.Manifest
		instanceName string
		want         string
	}{
		{
			name:         "instance name hostname",
			instanceName: "api-0",
			want:         "instance-id: api-0\nlocal-hostname: api-0\n",
		},
		{
			name: "cloud init hostname",
			manifest: config.Manifest{
				CloudInit: config.CloudInit{Hostname: "api.internal"},
			},
			instanceName: "api-0",
			want:         "instance-id: api-0\nlocal-hostname: api.internal\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := renderMetaData(tt.manifest, tt.instanceName); got != tt.want {
				t.Fatalf("renderMetaData = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderWriteFilesOrder(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name: "web",
		CloudInit: config.CloudInit{
			User: "ubuntu",
			WriteFiles: []config.WriteFile{
				{Path: "/etc/app.env", Content: "APP_ENV=prod\n"},
			},
		},
		ExtraHosts: map[string]string{
			"web": "10.10.0.2",
		},
	}

	files := renderWriteFiles(manifest, "web-0", familySystemd)
	gotPaths := ccFilePaths(files)
	wantPaths := []string{
		hostsFilePath,
		serialConsoleGrubPath,
		serialConsoleAutologinPath,
		"/etc/app.env",
	}
	assertStringSliceEqual(t, "renderWriteFiles paths", gotPaths, wantPaths)
}

func TestRenderRunCmdOrder(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		CloudInit: config.CloudInit{
			RunCmd: []string{"systemctl restart app"},
		},
		Mounts: []config.Mount{
			{Kind: config.MountKindVolume, VolumeName: "data", Target: "/var/lib/data"},
		},
	}

	cmds := renderRunCmd(manifest, familySystemd)
	assertStringSliceLen(t, "renderRunCmd", cmds, 3)
	wantPrefix := []string{"systemctl restart app", serialGettySystemdCmd}
	assertStringSliceEqual(t, "renderRunCmd prefix", cmds[:2], wantPrefix)
	assertContains(t, cmds[2], volumeMountErrorPrefix+"data")
}

func TestRenderWithExtraHosts(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name: "web",
		CloudInit: config.CloudInit{
			User: "ubuntu",
		},
		ExtraHosts: map[string]string{
			"web":   "10.10.0.2",
			"web-0": "10.10.0.2",
			"db":    "10.10.0.3",
			"db-0":  "10.10.0.3",
		},
	}

	userData, _, _ := Render(manifest, "web-0", 0)

	assertContains(t, userData,
		"manage_etc_hosts: false",
		"path: /etc/hosts",
		"10.10.0.2",
		"10.10.0.3",
	)
}

// The serial-getty runcmd assumes systemd. Previously it was emitted
// unconditionally, which meant Alpine guests ran failing `systemctl` chains.
// The renderer must now branch on the image family and emit neither the
// systemd drop-in nor the systemctl runcmd when the image looks like Alpine.
func TestRenderAlpineSkipsSystemdBits(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:    "web",
		ImageOS: config.ImageOSOpenRC,
		CloudInit: config.CloudInit{
			User:     "ubuntu",
			Packages: []string{"nginx"},
			RunCmd:   []string{"rc-service nginx start"},
		},
	}

	userData, _, _ := Render(manifest, "web-0", 0)

	assertOmits(t, userData,
		"/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
		"/etc/default/grub.d/99-serial-console.cfg",
		"systemctl enable serial-getty",
		"/bin/bash",
		"- adm",
	)

	assertContains(t, userData,
		"name: ubuntu",
		"- nginx",
		"- rc-service nginx start",
	)
}

// TestVolumeMountRunCmd_ReadWrite asserts the baseline behavior for
// writable named volumes: a `defaults,nofail` fstab entry is appended
// and an mkfs.ext4 guard runs on first boot only.
func TestVolumeMountRunCmd_ReadWrite(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Mounts: []config.Mount{
			{Kind: config.MountKindVolume, VolumeName: "data", Target: "/var/lib/data"},
		},
	}
	cmds := volumeMountRunCmd(manifest)
	assertStringSliceLen(t, "volumeMountRunCmd", cmds, 1)
	s := cmds[0]
	assertContains(t, s,
		volumeMkfsCommand,
		volumeFilesystem+" "+volumeFstabDefaultOpts,
		volumeMountErrorPrefix+"data",
	)
	assertOmits(t, s,
		volumeFilesystem+" "+volumeFstabReadOnlyOpts,
		"mount '/var/lib/data' || true",
	)
}

// TestVolumeMountRunCmd_ReadOnly pins the ro contract end-to-end on
// the guest side. Before the fix, cloud-init blindly ran mkfs.ext4
// against a readonly=on QEMU drive (which fails and spams errors)
// and wrote a `defaults,nofail` fstab line, so the guest mounted the
// disk writable despite the operator's compose `:ro` suffix. The
// renderer must skip mkfs and emit `ro,nofail`.
func TestVolumeMountRunCmd_ReadOnly(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Mounts: []config.Mount{
			{Kind: config.MountKindVolume, VolumeName: "shared", Target: "/srv/shared", ReadOnly: true},
		},
	}
	cmds := volumeMountRunCmd(manifest)
	assertStringSliceLen(t, "volumeMountRunCmd", cmds, 1)
	s := cmds[0]
	assertOmits(t, s, volumeMkfsCommand)
	assertContains(t, s, volumeFilesystem+" "+volumeFstabReadOnlyOpts)
	assertOmits(t, s, volumeFilesystem+" "+volumeFstabDefaultOpts)
}

// Conversely, when the image isn't Alpine, the existing systemd-oriented
// configuration must still be emitted.
func TestRenderSystemdIncludesSerialGetty(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name:    "web",
		ImageOS: config.ImageOSSystemd,
		CloudInit: config.CloudInit{
			User: "ubuntu",
		},
	}

	userData, _, _ := Render(manifest, "web-0", 0)

	assertContains(t, userData,
		"/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
		"systemctl enable serial-getty",
		"shell: /bin/bash",
	)
}

func TestRenderNetworkConfig(t *testing.T) {
	t.Parallel()

	manifest := config.Manifest{
		Name: "web",
		CloudInit: config.CloudInit{
			User: "ubuntu",
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: "230.0.0.1",
			MulticastPort:  12345,
			Subnet:         "10.10.0.0/24",
			InstanceIPs:    []string{"10.10.0.2", "10.10.0.3"},
			BaseMAC:        "52:54:00:ab:cd:00",
		},
	}

	_, _, networkConfig := Render(manifest, "web-0", 0)

	if networkConfig == "" {
		t.Fatal("expected non-empty network config")
	}
	assertContains(t, networkConfig, "10.10.0.2/24", "52:54:00:ab:cd:00")
}

func ccFilePaths(files []ccFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}
