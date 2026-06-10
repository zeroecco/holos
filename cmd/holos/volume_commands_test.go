package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

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
