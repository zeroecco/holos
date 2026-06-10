package compose

import (
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testParseVolumeName      = "data"
	testParseVolumeSize      = "5G"
	testParseVolumeSizeBytes = 5 * (1 << 30)
	testParseBindSource      = "./data"
	testParseAbsBindSource   = "/srv/data"
	testParseVolumeTarget    = "/var/lib/db"
)

func TestParseVolume(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	mount, err := parseVolume(testParseBindSource+":"+testParseVolumeTarget+":"+volumeModeReadOnly, dir, nil)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.Kind != config.MountKindBind {
		t.Fatalf("expected bind kind, got %q", mount.Kind)
	}
	if !filepath.IsAbs(mount.Source) {
		t.Fatalf("expected absolute source, got %s", mount.Source)
	}
	if mount.Target != testParseVolumeTarget {
		t.Fatalf("expected target %s, got %s", testParseVolumeTarget, mount.Target)
	}
	if !mount.ReadOnly {
		t.Fatal("expected read-only mount")
	}
}

func TestParseVolume_Named(t *testing.T) {
	t.Parallel()

	declared := map[string]Volume{
		testParseVolumeName: {Size: testParseVolumeSize},
	}

	mount, err := parseVolume(testParseVolumeName+":"+testParseVolumeTarget, t.TempDir(), declared)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.Kind != config.MountKindVolume {
		t.Fatalf("expected volume kind, got %q", mount.Kind)
	}
	if mount.VolumeName != testParseVolumeName {
		t.Fatalf("expected volume_name %s, got %q", testParseVolumeName, mount.VolumeName)
	}
	if mount.SizeBytes != testParseVolumeSizeBytes {
		t.Fatalf("expected size %d bytes, got %d", testParseVolumeSizeBytes, mount.SizeBytes)
	}
	if mount.Source != "" {
		t.Fatalf("named volume should have no host source, got %q", mount.Source)
	}
}

func TestBindVolumeMount(t *testing.T) {
	t.Parallel()

	mount := bindVolumeMount(testParseAbsBindSource, testParseVolumeTarget, true)
	assertMount(t, "bind", mount, testMountWant{
		kind:     config.MountKindBind,
		source:   testParseAbsBindSource,
		target:   testParseVolumeTarget,
		readOnly: true,
	})
}

func TestNamedVolumeMount(t *testing.T) {
	t.Parallel()

	mount := namedVolumeMount(testParseVolumeName, testParseVolumeTarget, testParseVolumeSizeBytes, true)
	assertMount(t, "volume", mount, testMountWant{
		kind:       config.MountKindVolume,
		target:     testParseVolumeTarget,
		volumeName: testParseVolumeName,
		sizeBytes:  testParseVolumeSizeBytes,
		readOnly:   true,
	})
}

// TestParseVolume_RejectsUnknownMode pins the allow-list contract on
// the third ":mode" field. Before this change anything that wasn't
// exactly "ro" silently parsed as read-write, so a typo like
// `:readonly` or docker-compose's `:rw,Z` delivered a writable mount
// without any signal to the operator. The fix is to fail loudly for
// both bind mounts and named volumes; the test exercises both paths
// because the code branches on the declared map before validation.
func TestParseVolume_RejectsUnknownMode(t *testing.T) {
	t.Parallel()

	declared := map[string]Volume{testParseVolumeName: {Size: "1G"}}

	cases := []struct {
		name string
		spec string
		decl map[string]Volume
	}{
		{"bind readonly-typo", testParseBindSource + ":" + testParseVolumeTarget + ":readonly", nil},
		{"bind r0-typo", testParseBindSource + ":" + testParseVolumeTarget + ":r0", nil},
		{"bind docker-style-z", testParseBindSource + ":" + testParseVolumeTarget + ":Z", nil},
		{"named readonly-typo", testParseVolumeName + ":" + testParseVolumeTarget + ":readonly", declared},
		{"named empty-mode", testParseVolumeName + ":" + testParseVolumeTarget + ":", declared},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseVolume(tc.spec, t.TempDir(), tc.decl)
			assertErrorContains(t, err, "unknown mode")
		})
	}
}

func TestSplitVolumeSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    string
		want    []string
		wantErr string
	}{
		{
			name: "source and target",
			spec: testParseBindSource + ":" + testParseVolumeTarget,
			want: []string{testParseBindSource, testParseVolumeTarget},
		},
		{
			name: "source target and mode",
			spec: testParseBindSource + ":" + testParseVolumeTarget + ":" + volumeModeReadOnly,
			want: []string{testParseBindSource, testParseVolumeTarget, volumeModeReadOnly},
		},
		{
			name: "preserves remaining separators in mode field",
			spec: testParseBindSource + ":" + testParseVolumeTarget + ":rw,Z",
			want: []string{testParseBindSource, testParseVolumeTarget, "rw,Z"},
		},
		{
			name:    "missing target",
			spec:    testParseVolumeName,
			wantErr: "source:target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := splitVolumeSpec(tt.spec)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("splitVolumeSpec: %v", err)
			}
			assertStringSliceEqual(t, "splitVolumeSpec", got, tt.want)
		})
	}
}

func TestParseVolumeMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    string
		parts   []string
		want    bool
		wantErr string
	}{
		{name: "omitted", spec: "./data:/var/lib/db", parts: []string{testParseBindSource, testParseVolumeTarget}},
		{name: "read only", spec: "./data:/var/lib/db:ro", parts: []string{testParseBindSource, testParseVolumeTarget, volumeModeReadOnly}, want: true},
		{name: "read write", spec: "./data:/var/lib/db:rw", parts: []string{testParseBindSource, testParseVolumeTarget, volumeModeReadWrite}},
		{name: "unknown", spec: "./data:/var/lib/db:readonly", parts: []string{testParseBindSource, testParseVolumeTarget, "readonly"}, wantErr: "unknown mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVolumeMode(tt.spec, tt.parts)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("parseVolumeMode: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseVolumeMode = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveBindVolumeSource(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	absSource := filepath.Join(t.TempDir(), testParseVolumeName)

	tests := []struct {
		name    string
		source  string
		want    string
		wantErr string
	}{
		{
			name:   "absolute path",
			source: absSource,
			want:   absSource,
		},
		{
			name:   "explicit relative path",
			source: testParseBindSource,
			want:   filepath.Join(baseDir, testParseVolumeName),
		},
		{
			name:   "explicit parent-relative path",
			source: "../data",
			want:   filepath.Join(baseDir, "..", testParseVolumeName),
		},
		{
			name:    "bare identifier",
			source:  testParseVolumeName,
			wantErr: "not a declared top-level volume",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveBindVolumeSource(tt.source, baseDir)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("resolveBindVolumeSource: %v", err)
			}
			want, err := filepath.Abs(tt.want)
			if err != nil {
				t.Fatalf("filepath.Abs(%q): %v", tt.want, err)
			}
			if got != want {
				t.Fatalf("resolveBindVolumeSource = %q, want %q", got, want)
			}
		})
	}
}

// TestParseVolume_AcceptsExplicitRW covers the symmetric side: `:rw`
// is equivalent to no mode suffix and must not be rejected by the
// new allow-list. Users migrating from docker-compose files that
// spell the mode out shouldn't need to strip it.
func TestParseVolume_AcceptsExplicitRW(t *testing.T) {
	t.Parallel()

	mount, err := parseVolume(testParseBindSource+":"+testParseVolumeTarget+":"+volumeModeReadWrite, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if mount.ReadOnly {
		t.Fatalf("`:rw` must parse as writable, got ReadOnly=true")
	}
}
