package images

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testRefAlpineName = "alpine"
	testRefAlpineTag  = "3.21"
	testRefAlpineURL  = "https://example.com/alpine.qcow2"
	testRefCacheRoot  = "/cache"
)

func testRefAlpineImage(format string) *Image {
	return &Image{Name: testRefAlpineName, Tag: testRefAlpineTag, URL: testRefAlpineURL, Format: format}
}

func TestCacheFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		img  *Image
		want string
	}{
		{
			name: "qcow2",
			img:  testRefAlpineImage(config.ImageFormatQCOW2),
			want: "alpine-3.21-e4c54dd5.qcow2",
		},
		{
			name: "raw",
			img:  testRefAlpineImage(config.ImageFormatRaw),
			want: "alpine-3.21-e4c54dd5.raw",
		},
		{
			name: "empty format defaults to qcow2",
			img:  testRefAlpineImage(""),
			want: "alpine-3.21-e4c54dd5.qcow2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheFilename(tt.img); got != tt.want {
				t.Fatalf("cacheFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCachePathUsesStableCacheLayout(t *testing.T) {
	t.Parallel()

	img := testRefAlpineImage(config.ImageFormatQCOW2)
	if got, want := cachePath(testRefCacheRoot, img), "/cache/alpine-3.21-e4c54dd5.qcow2"; got != want {
		t.Fatalf("cachePath = %q, want %q", got, want)
	}
}

func TestParseRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		name string
		tag  string
	}{
		{"alpine", "alpine", ""},
		{"ubuntu:noble", "ubuntu", "noble"},
		{"debian:12", "debian", "12"},
		{"registry.local/team/image:stable", "registry.local/team/image", "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			name, tag := parseRef(tt.ref)
			if name != tt.name || tag != tt.tag {
				t.Fatalf("parseRef(%q) = (%q, %q), want (%q, %q)", tt.ref, name, tag, tt.name, tt.tag)
			}
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "absolute qcow2", ref: "/abs/base.qcow2", want: true},
		{name: "relative raw", ref: "./base.raw", want: true},
		{name: "parent img", ref: "../base.img", want: true},
		{name: "bare qcow2", ref: "base.qcow2", want: true},
		{name: "bare raw", ref: "base.raw", want: true},
		{name: "bare img", ref: "base.img", want: true},
		{name: "registry tag", ref: "ubuntu:noble"},
		{name: "registry tag with image suffix", ref: "ubuntu:noble.img"},
		{name: "registry host tag", ref: "registry.local/vm:tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLocalPath(tt.ref); got != tt.want {
				t.Fatalf("isLocalPath(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestInferFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "raw extension", path: "base.raw", want: config.ImageFormatRaw},
		{name: "qcow2 extension", path: "base.qcow2", want: config.ImageFormatQCOW2},
		{name: "img defaults to qcow2", path: "base.img", want: config.ImageFormatQCOW2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferFormat(tt.path); got != tt.want {
				t.Fatalf("inferFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
