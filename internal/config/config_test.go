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

	if manifest.APIVersion != "holosteric/v1alpha1" {
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
