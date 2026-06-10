package compose

import (
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestResolveImagePath(t *testing.T) {
	t.Parallel()

	baseDir := filepath.Join(t.TempDir(), "project")
	absPath := filepath.Join(t.TempDir(), "base.qcow2")

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "relative", path: "base.qcow2", want: filepath.Join(baseDir, "base.qcow2")},
		{name: "absolute", path: absPath, want: absPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveImagePath(baseDir, tt.path); got != tt.want {
				t.Fatalf("resolveImagePath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateImagePathRejectsDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := validateImagePath("./image-dir", dir)
	assertErrorContains(t, err, "is a directory")
}

func TestResolveImageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pulledFormat   string
		explicitFormat string
		want           string
	}{
		{name: "uses pulled format", pulledFormat: config.ImageFormatQCOW2, want: config.ImageFormatQCOW2},
		{name: "explicit overrides pulled", pulledFormat: config.ImageFormatQCOW2, explicitFormat: config.ImageFormatRaw, want: config.ImageFormatRaw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveImageFormat(tt.pulledFormat, tt.explicitFormat); got != tt.want {
				t.Fatalf("resolveImageFormat = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImageOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ref        string
		explicitOS string
		want       string
	}{
		{name: "explicit wins", ref: "alpine", explicitOS: config.ImageOSSystemd, want: config.ImageOSSystemd},
		{name: "resolver metadata", ref: "alpine", want: config.ImageOSOpenRC},
		{name: "default", ref: "custom", want: config.ImageOSSystemd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveImageOS(tt.ref, tt.explicitOS, composeTestImages)
			if got != tt.want {
				t.Fatalf("resolveImageOS = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImageUsesExplicitFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	imagePath := writeTestImage(t, dir)

	gotPath, gotFormat, err := resolveImage("./base.qcow2", config.ImageFormatRaw, dir, filepath.Join(dir, "cache"), composeTestImages)
	if err != nil {
		t.Fatalf("resolveImage: %v", err)
	}
	if gotPath != imagePath {
		t.Fatalf("resolveImage path = %q, want %q", gotPath, imagePath)
	}
	if gotFormat != config.ImageFormatRaw {
		t.Fatalf("resolveImage format = %q, want %q", gotFormat, config.ImageFormatRaw)
	}
}
