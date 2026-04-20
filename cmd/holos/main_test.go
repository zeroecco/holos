package main

import (
	"strings"
	"testing"
)

func TestParseMemoryMB(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"512", 512, false},
		{"512M", 512, false},
		{"512m", 512, false},
		{"512MB", 512, false},
		{"2G", 2048, false},
		{"2GB", 2048, false},
		{"2g", 2048, false},
		{"1T", 1024 * 1024, false},
		{"4096K", 4, false},
		{"  1G  ", 1024, false},
		{"", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"512K", 0, true}, // 512 KiB rounds to 0 MB; rejected
		{"0", 0, true},
	}
	for _, c := range cases {
		got, err := parseMemoryMB(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMemoryMB(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMemoryMB(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMemoryMB(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGenerateRunName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		image      string
		dockerfile string
		wantPrefix string
	}{
		{"ubuntu:noble", "", "ubuntu-noble-"},
		{"alpine", "", "alpine-"},
		{"./images/my-image.qcow2", "", "my-image-"},
		{"/var/lib/libvirt/images/web.raw", "", "web-"},
		{"", "./Dockerfile", "dockerfile-"},
		{"REGISTRY/Foo_Bar:1.0", "", "foo-bar-"},
	}
	for _, c := range cases {
		got := generateRunName(c.image, c.dockerfile)
		if !strings.HasPrefix(got, c.wantPrefix) {
			t.Errorf("generateRunName(%q, %q) = %q, want prefix %q",
				c.image, c.dockerfile, got, c.wantPrefix)
		}
		if !runNamePattern.MatchString(got) {
			t.Errorf("generateRunName(%q, %q) = %q, fails compose name validation",
				c.image, c.dockerfile, got)
		}
		if len(got) > 63 {
			t.Errorf("generateRunName(%q, %q) = %q (len %d), exceeds 63-char limit",
				c.image, c.dockerfile, got, len(got))
		}
	}
}

func TestGenerateRunNameUnique(t *testing.T) {
	t.Parallel()

	// Repeated invocations on the same image should produce distinct
	// names (random suffix). We're not asserting a strong uniqueness
	// guarantee here — just that the suffix isn't a constant.
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		seen[generateRunName("alpine", "")] = true
	}
	if len(seen) < 8 {
		t.Errorf("expected diverse run names across 16 calls, got only %d unique", len(seen))
	}
}

func TestGenerateRunNameLongImage(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 200)
	got := generateRunName(long, "")
	if len(got) > 63 {
		t.Fatalf("generateRunName(<200 a's>) = %q (len %d), exceeds 63-char limit", got, len(got))
	}
	if !runNamePattern.MatchString(got) {
		t.Errorf("generateRunName(<200 a's>) = %q, fails compose name validation", got)
	}
}

func TestStringListAppends(t *testing.T) {
	t.Parallel()

	var list stringList
	for _, v := range []string{"8080:80", "9090:90", "5432:5432"} {
		if err := list.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	if len(list) != 3 || list[0] != "8080:80" || list[2] != "5432:5432" {
		t.Errorf("stringList accumulation broken: %v", list)
	}
	if got := list.String(); got != "8080:80,9090:90,5432:5432" {
		t.Errorf("String() = %q", got)
	}
}
