package cloudinit

import (
	"strings"
	"testing"

	"github.com/rich/holosteric/internal/config"
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
		`hostname: "api-0"`,
		`name: "ubuntu"`,
		`- "curl"`,
		`path: "/etc/api.env"`,
		"PORT=8080",
		`- "systemctl restart api"`,
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
	if !strings.Contains(userData, `path: "/etc/hosts"`) {
		t.Fatal("expected /etc/hosts write_file entry")
	}
	if !strings.Contains(userData, "10.10.0.2") {
		t.Fatalf("expected IP in hosts file content:\n%s", userData)
	}
	if !strings.Contains(userData, "10.10.0.3") {
		t.Fatalf("expected db IP in hosts file content:\n%s", userData)
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
