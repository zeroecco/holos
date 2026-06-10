package qemu

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestVirtfsOption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mount config.Mount
		want  string
	}{
		{
			name:  "read write",
			mount: config.Mount{Source: "/srv/app", Target: "/var/lib/app"},
			want:  "local,path=/srv/app,mount_tag=share2-var-lib-app,security_model=none",
		},
		{
			name:  "readonly escaped source",
			mount: config.Mount{Source: "/srv/app,data", Target: "/data", ReadOnly: true},
			want:  "local,path=/srv/app,,data,mount_tag=share2-data,security_model=none,readonly=on",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := virtfsOption(2, tt.mount); got != tt.want {
				t.Fatalf("virtfsOption() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMountTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "absolute path", target: "/var/lib/app", want: "share2-var-lib-app"},
		{name: "trailing slash", target: "/var/lib/app/", want: "share2-var-lib-app"},
		{name: "relative path", target: "var/lib/app", want: "share2-var-lib-app"},
		{name: "root", target: "/", want: "share2-root"},
		{name: "dot", target: ".", want: "share2-root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mountTag(2, tt.target); got != tt.want {
				t.Fatalf("mountTag(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestMountTagTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "trailing slash", target: "/var/lib/app/", want: "var-lib-app"},
		{name: "relative path", target: "var/lib/app", want: "var-lib-app"},
		{name: "root", target: "/", want: mountTagRootTarget},
		{name: "dot", target: ".", want: mountTagRootTarget},
		{name: "empty", target: "", want: mountTagRootTarget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mountTagTarget(tt.target); got != tt.want {
				t.Fatalf("mountTagTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
