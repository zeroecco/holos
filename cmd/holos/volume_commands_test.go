package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

const testVolumeCommandProject = "demo"

func TestIsVolumeRemoveCommand(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{"rm", "remove"} {
		if !isVolumeRemoveCommand(arg) {
			t.Fatalf("isVolumeRemoveCommand(%q) = false, want true", arg)
		}
	}
	if isVolumeRemoveCommand("list") {
		t.Fatal("isVolumeRemoveCommand(list) = true, want false")
	}
}

func TestRunVolumeRemoveRemovesDetachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	backing := filepath.Join(stateDir, "volumes", testVolumeCommandProject, "data.qcow2")
	if err := os.MkdirAll(filepath.Dir(backing), 0o700); err != nil {
		t.Fatalf("mkdir volume dir: %v", err)
	}
	if err := os.WriteFile(backing, []byte("volume"), 0o600); err != nil {
		t.Fatalf("write volume: %v", err)
	}

	err := runVolumeRemove([]string{"--state-dir", stateDir, testVolumeCommandProject, "data"})
	if err != nil {
		t.Fatalf("runVolumeRemove: %v", err)
	}
	if _, err := os.Stat(backing); !os.IsNotExist(err) {
		t.Fatalf("removed backing stat err = %v, want not exist", err)
	}
}

func TestRunVolumeRemoveRequiresProjectAndVolume(t *testing.T) {
	t.Parallel()

	err := runVolumeRemove(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: holos volumes rm") {
		t.Fatalf("runVolumeRemove(nil) err = %v, want usage", err)
	}
}

func TestRunVolumeExportCopiesDetachedVolume(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	backing := filepath.Join(stateDir, "volumes", testVolumeCommandProject, "data.qcow2")
	if err := os.MkdirAll(filepath.Dir(backing), 0o700); err != nil {
		t.Fatalf("mkdir volume dir: %v", err)
	}
	if err := os.WriteFile(backing, []byte("volume"), 0o600); err != nil {
		t.Fatalf("write volume: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "data.qcow2")

	err := runVolumeExport([]string{"--state-dir", stateDir, testVolumeCommandProject, "data", destination})
	if err != nil {
		t.Fatalf("runVolumeExport: %v", err)
	}
	payload, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(payload) != "volume" {
		t.Fatalf("export payload = %q, want volume", string(payload))
	}
}

func TestRunVolumeExportRequiresProjectVolumeAndPath(t *testing.T) {
	t.Parallel()

	err := runVolumeExport(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: holos volumes export") {
		t.Fatalf("runVolumeExport(nil) err = %v, want usage", err)
	}
}

func TestFilterVolumesByProject(t *testing.T) {
	t.Parallel()

	volumes := []runtime.VolumeInfo{
		{Project: "api", Name: "cache"},
		{Project: "web", Name: "data"},
	}
	filtered := filterVolumesByProject(volumes, "web")
	if len(filtered) != 1 || filtered[0].Name != "data" {
		t.Fatalf("filtered volumes = %+v, want web/data only", filtered)
	}
}

func TestWriteVolumesTable(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := writeVolumesTable(&out, []runtime.VolumeInfo{
		{
			Project:   "demo",
			Name:      "data",
			SizeBytes: 10485760,
			Path:      "/state/volumes/demo/data.qcow2",
			Attachments: []runtime.VolumeAttachmentInfo{
				{Instance: "web-0", Status: runtime.InstanceStatusRunning},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeVolumesTable: %v", err)
	}
	got := out.String()
	for _, want := range []string{"PROJECT", "VOLUME", "SIZE_BYTES", "ATTACHED_TO", "demo", "data", "10485760", "web-0:running", "/state/volumes/demo/data.qcow2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("volumes table missing %q:\n%s", want, got)
		}
	}
}

func TestWriteVolumesTableEmpty(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := writeVolumesTable(&out, nil); err != nil {
		t.Fatalf("writeVolumesTable: %v", err)
	}
	if got, want := out.String(), "no named volumes\n"; got != want {
		t.Fatalf("empty table = %q, want %q", got, want)
	}
}
