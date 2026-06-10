package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
	"github.com/zeroecco/holos/internal/runtime"
)

const (
	testInspectProject  = "demo"
	testInspectService  = "web"
	testInspectInstance = "web-0"
)

func TestInspectTargetDefaultsToComposeProject(t *testing.T) {
	t.Parallel()

	project := &compose.Project{Name: testInspectProject}
	if got := inspectTarget(nil, project); got != testInspectProject {
		t.Fatalf("inspectTarget default = %q, want %q", got, testInspectProject)
	}
	if got := inspectTarget([]string{testInspectInstance}, project); got != testInspectInstance {
		t.Fatalf("inspectTarget explicit = %q, want %q", got, testInspectInstance)
	}
}

func TestInspectProjectDocumentIncludesRecordVolumesAndManifests(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)
	record := testProjectRecord(testInspectProject, runtime.ServiceRecord{Name: testInspectService})
	record.Volumes = []runtime.VolumeRecord{{Name: "data", SizeBytes: 10 << 20}}
	if err := writeProjectRecord(stateDir, record); err != nil {
		t.Fatalf("write project record: %v", err)
	}
	writeInspectVolume(t, stateDir, "data", "volume")
	project := inspectComposeProject()

	doc, err := inspectTargetDocument(manager, project, testInspectProject)
	if err != nil {
		t.Fatalf("inspectTargetDocument: %v", err)
	}
	if doc.Kind != "project" || doc.Project != testInspectProject || doc.Record == nil {
		t.Fatalf("project doc identity = %+v", doc)
	}
	if _, ok := doc.Manifests[testInspectService]; !ok {
		t.Fatalf("project doc manifests = %+v, missing %q", doc.Manifests, testInspectService)
	}
	if len(doc.Volumes) != 1 || doc.Volumes[0].Name != "data" {
		t.Fatalf("project doc volumes = %+v, want data", doc.Volumes)
	}
}

func TestInspectInstanceDocumentIncludesManifestAndQEMUArgs(t *testing.T) {
	t.Parallel()

	workDir := filepath.Join(t.TempDir(), testInspectInstance)
	instance := runtime.InstanceRecord{
		Name:        testInspectInstance,
		Index:       0,
		WorkDir:     workDir,
		OverlayPath: filepath.Join(workDir, "root.qcow2"),
		SeedPath:    filepath.Join(workDir, "seed.iso"),
		LogPath:     filepath.Join(workDir, "console.log"),
		SerialPath:  filepath.Join(workDir, "serial.sock"),
		QMPPath:     filepath.Join(workDir, "qmp.sock"),
		SSHPort:     2222,
		Ports: []qemu.PortMapping{{
			HostAddr:  "127.0.0.1",
			HostPort:  8080,
			GuestPort: 80,
			Protocol:  config.ProtocolTCP,
		}},
	}
	service := runtime.ServiceRecord{Name: testInspectService, Instances: []runtime.InstanceRecord{instance}}
	record := testProjectRecord(testInspectProject, service)

	doc, err := inspectInstanceDocument(inspectComposeProject(), record, service, instance)
	if err != nil {
		t.Fatalf("inspectInstanceDocument: %v", err)
	}
	if doc.Kind != "instance" || doc.Instance == nil || doc.Instance.Name != testInspectInstance {
		t.Fatalf("instance doc identity = %+v", doc)
	}
	if doc.Manifest == nil || doc.Manifest.Name != testInspectService {
		t.Fatalf("instance doc manifest = %+v, want %q", doc.Manifest, testInspectService)
	}
	if !slices.Contains(doc.QEMUArgs, "-name") || !slices.Contains(doc.QEMUArgs, testInspectInstance) {
		t.Fatalf("qemu args = %+v, want instance name", doc.QEMUArgs)
	}
}

func TestInspectTargetDocumentScopesToComposeProject(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	manager := runtime.NewManager(stateDir)
	if err := writeProjectRecord(stateDir, testProjectRecord(testInspectProject)); err != nil {
		t.Fatalf("write project record: %v", err)
	}

	_, err := inspectTargetDocument(manager, &compose.Project{Name: testInspectProject}, "missing-0")
	if err == nil {
		t.Fatal("inspectTargetDocument missing target err = nil, want error")
	}
}

func inspectComposeProject() *compose.Project {
	return &compose.Project{
		Name: testInspectProject,
		Services: map[string]config.Manifest{
			testInspectService: {
				Name:        testInspectService,
				Image:       "/tmp/base.qcow2",
				ImageFormat: config.ImageFormatQCOW2,
				VM: config.VMConfig{
					VCPU:     1,
					MemoryMB: 512,
					Machine:  "q35",
					CPUModel: "host",
				},
				Network: config.NetworkConfig{Mode: config.DefaultNetworkMode},
			},
		},
	}
}

func writeInspectVolume(t *testing.T, stateDir, name, content string) {
	t.Helper()

	path := filepath.Join(stateDir, "volumes", testInspectProject, name+".qcow2")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir volume dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write volume: %v", err)
	}
}
