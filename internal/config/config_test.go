package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestDefaultsAndPathResolution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "images"), 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	manifestPath := filepath.Join(root, "service.json")
	content := `{
  "name": "api",
  "image": "./images/base.qcow2",
  "ports": [{"guest_port": 8080}],
  "mounts": [{"source": "./data", "target": "/var/lib/api"}],
  "cloud_init": {
    "write_files": [{"path": "/etc/api.env", "content": "MODE=prod\n"}]
  }
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	if manifest.APIVersion != "holos/v1alpha1" {
		t.Fatalf("unexpected api version: %s", manifest.APIVersion)
	}
	if manifest.Replicas != 1 {
		t.Fatalf("unexpected replicas: %d", manifest.Replicas)
	}
	if manifest.Ports[0].Protocol != "tcp" {
		t.Fatalf("unexpected protocol: %s", manifest.Ports[0].Protocol)
	}
	if manifest.CloudInit.User != "ubuntu" {
		t.Fatalf("unexpected cloud-init user: %s", manifest.CloudInit.User)
	}
	if !filepath.IsAbs(manifest.Image) {
		t.Fatalf("expected absolute image path, got %s", manifest.Image)
	}
	if !filepath.IsAbs(manifest.Mounts[0].Source) {
		t.Fatalf("expected absolute mount source, got %s", manifest.Mounts[0].Source)
	}
}

func TestLoadManifestRejectsInvalidServiceName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "service.json")
	content := `{"name": "INVALID_NAME", "image": "/tmp/base.qcow2"}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := LoadManifest(manifestPath); err == nil {
		t.Fatal("expected invalid service name error")
	}
}

func TestValidateRejectsInvalidCloudInitUser(t *testing.T) {
	t.Parallel()

	m := Manifest{
		Name:        "api",
		Replicas:    1,
		Image:       "/tmp/base.qcow2",
		ImageFormat: "qcow2",
		VM:          VMConfig{VCPU: 1, MemoryMB: 512},
		Network:     NetworkConfig{Mode: "user"},
		CloudInit:   CloudInit{User: "bad user"},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid cloud_init.user error")
	}
}

func TestValidateRejectsInvalidPCIAddress(t *testing.T) {
	t.Parallel()

	m := Manifest{
		Name:        "gpu",
		Replicas:    1,
		Image:       "/tmp/base.qcow2",
		ImageFormat: "qcow2",
		VM:          VMConfig{VCPU: 1, MemoryMB: 512},
		Network:     NetworkConfig{Mode: "user"},
		CloudInit:   CloudInit{User: "ubuntu"},
		Devices:     []Device{{PCI: "01:00.8"}},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("expected invalid PCI address error")
	}
}

func TestValidateUserName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"ubuntu", "_svc", "build-user", "u123"} {
		if err := ValidateUserName(name); err != nil {
			t.Fatalf("ValidateUserName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "Ubuntu", "123user", "bad user", "bad/user", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if err := ValidateUserName(name); err == nil {
			t.Fatalf("ValidateUserName(%q) succeeded, want error", name)
		}
	}
}

func TestValidatePCIAddress(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"0000:01:00.0", "abcd:ef:12.7"} {
		if err := ValidatePCIAddress(addr); err != nil {
			t.Fatalf("ValidatePCIAddress(%q): %v", addr, err)
		}
	}
	for _, addr := range []string{"", "01:00.0", "0000:01:00.8", "0000:01:00", "0000:1:00.0"} {
		if err := ValidatePCIAddress(addr); err == nil {
			t.Fatalf("ValidatePCIAddress(%q) succeeded, want error", addr)
		}
	}
}
