package cloudinit

import (
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

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
					Permissions: "0644",
					Owner:       "root:root",
				},
			},
		},
	}

	userData, metaData, _ := Render(manifest, "api-0", 0)

	for _, needle := range []string{
		"#cloud-config",
		"hostname: api-0",
		"name: ubuntu",
		"- curl",
		"path: /etc/api.env",
		"PORT=8080",
		"- systemctl restart api",
	} {
		if !strings.Contains(userData, needle) {
			t.Fatalf("expected user-data to contain %q, got:\n%s", needle, userData)
		}
	}

	if !strings.Contains(metaData, "instance-id: api-0") {
		t.Fatalf("unexpected meta-data:\n%s", metaData)
	}
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

	if !strings.Contains(userData, "manage_etc_hosts: false") {
		t.Fatal("expected manage_etc_hosts: false with extra hosts")
	}
	if !strings.Contains(userData, "path: /etc/hosts") {
		t.Fatal("expected /etc/hosts write_file entry")
	}
	if !strings.Contains(userData, "10.10.0.2") {
		t.Fatalf("expected IP in hosts file content:\n%s", userData)
	}
	if !strings.Contains(userData, "10.10.0.3") {
		t.Fatalf("expected db IP in hosts file content:\n%s", userData)
	}
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

	for _, forbidden := range []string{
		"/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
		"/etc/default/grub.d/99-serial-console.cfg",
		"systemctl enable serial-getty",
		"/bin/bash",
		"- adm",
	} {
		if strings.Contains(userData, forbidden) {
			t.Fatalf("expected Alpine user-data to omit %q, got:\n%s", forbidden, userData)
		}
	}

	for _, required := range []string{
		"name: ubuntu",
		"- nginx",
		"- rc-service nginx start",
	} {
		if !strings.Contains(userData, required) {
			t.Fatalf("expected Alpine user-data to contain %q, got:\n%s", required, userData)
		}
	}
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
	if len(cmds) != 1 {
		t.Fatalf("expected 1 runcmd, got %d: %v", len(cmds), cmds)
	}
	s := cmds[0]
	if !strings.Contains(s, "mkfs.ext4") {
		t.Fatalf("writable volume should mkfs: %s", s)
	}
	if !strings.Contains(s, "ext4 defaults,nofail") {
		t.Fatalf("writable volume should use defaults,nofail fstab opts: %s", s)
	}
	if strings.Contains(s, "ext4 ro,nofail") {
		t.Fatalf("writable volume should not be marked ro in fstab: %s", s)
	}
	if strings.Contains(s, "mount '/var/lib/data' || true") {
		t.Fatalf("volume mount failures should be visible, got swallowed command: %s", s)
	}
	if !strings.Contains(s, "holos: failed to mount volume data") {
		t.Fatalf("volume mount command should emit a clear holos error: %s", s)
	}
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
	if len(cmds) != 1 {
		t.Fatalf("expected 1 runcmd, got %d: %v", len(cmds), cmds)
	}
	s := cmds[0]
	if strings.Contains(s, "mkfs.ext4") {
		t.Fatalf("read-only volume must not attempt mkfs: %s", s)
	}
	if !strings.Contains(s, "ext4 ro,nofail") {
		t.Fatalf("read-only volume should use ro,nofail fstab opts: %s", s)
	}
	if strings.Contains(s, "ext4 defaults,nofail") {
		t.Fatalf("read-only volume should not fall back to defaults,nofail: %s", s)
	}
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

	for _, required := range []string{
		"/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
		"systemctl enable serial-getty",
		"shell: /bin/bash",
	} {
		if !strings.Contains(userData, required) {
			t.Fatalf("expected systemd user-data to contain %q, got:\n%s", required, userData)
		}
	}
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
	if !strings.Contains(networkConfig, "10.10.0.2/24") {
		t.Fatalf("expected IP in network config:\n%s", networkConfig)
	}
	if !strings.Contains(networkConfig, "52:54:00:ab:cd:00") {
		t.Fatalf("expected MAC in network config:\n%s", networkConfig)
	}
}
