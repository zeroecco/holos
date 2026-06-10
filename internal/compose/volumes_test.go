package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testComposeVolumeName       = "data"
	testComposeVolumeTarget     = "/var/lib/data"
	testComposeBindSource       = "./data"
	testComposeBindTarget       = "/mnt/data"
	testComposeMetadataSize     = "20G"
	testComposeMetadataSizeByte = 20 * (1 << 30)
	testComposeLongSizeByte     = 5 * (1 << 30)
)

type testMountWant struct {
	kind       string
	source     string
	target     string
	volumeName string
	sizeBytes  int64
	readOnly   bool
}

func assertMount(t *testing.T, name string, got config.Mount, want testMountWant) {
	t.Helper()

	if got.Kind != want.kind ||
		got.Source != want.source ||
		got.Target != want.target ||
		got.VolumeName != want.volumeName ||
		got.SizeBytes != want.sizeBytes ||
		got.ReadOnly != want.readOnly {
		t.Fatalf("%s mount = %+v, want %+v", name, got, want)
	}
}

func TestResolveAcceptsComposeVolumeMetadataSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: volumemeta
services:
  db:
    image: ./base.qcow2
    volumes:
      - data:/var/lib/data
volumes:
  data:
    name: external-data
    driver: local
    driver_opts:
      type: none
    external:
      name: shared-data
    labels:
      com.example.role: data
    size: 20G
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Volumes[testComposeVolumeName].SizeBytes; got != testComposeMetadataSizeByte {
		t.Fatalf("volume size = %d, want %s", got, testComposeMetadataSize)
	}
}

func TestResolveAcceptsComposeServiceLongVolumeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	bindDir := filepath.Join(dir, testComposeVolumeName)
	if err := os.MkdirAll(bindDir, 0o755); err != nil {
		t.Fatalf("mkdir bind: %v", err)
	}
	yamlDoc := `
name: longvolumes
services:
  db:
    image: ./base.qcow2
    volumes:
      - type: volume
        source: data
        target: /var/lib/data
        read_only: true
        volume:
          nocopy: true
      - type: bind
        source: ./data
        target: /mnt/data
        bind:
          create_host_path: true
volumes:
  data:
    size: 5G
`
	project := resolveTestCompose(t, dir, yamlDoc)
	mounts := project.Services[testComposeDBService].Mounts
	if len(mounts) != 2 {
		t.Fatalf("mounts len = %d, want 2: %#v", len(mounts), mounts)
	}
	assertMount(t, "volume", mounts[0], testMountWant{
		kind:       config.MountKindVolume,
		target:     testComposeVolumeTarget,
		volumeName: testComposeVolumeName,
		sizeBytes:  testComposeLongSizeByte,
		readOnly:   true,
	})
	assertMount(t, "bind", mounts[1], testMountWant{
		kind:   config.MountKindBind,
		source: bindDir,
		target: testComposeBindTarget,
	})
}

func TestComposeVolumeType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec ComposeVolume
		want string
	}{
		{name: "default", want: composeVolumeTypeVolume},
		{name: "bind", spec: ComposeVolume{Type: composeVolumeTypeBind}, want: composeVolumeTypeBind},
		{name: "tmpfs", spec: ComposeVolume{Type: composeVolumeTypeTmpfs}, want: composeVolumeTypeTmpfs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := composeVolumeType(tt.spec); got != tt.want {
				t.Fatalf("composeVolumeType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseComposeVolumeRequiresLongVolumeEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec ComposeVolume
	}{
		{name: "missing both", spec: ComposeVolume{Type: composeVolumeTypeVolume}},
		{name: "source only", spec: ComposeVolume{Type: composeVolumeTypeVolume, Source: testComposeVolumeName}},
		{name: "target only", spec: ComposeVolume{Type: composeVolumeTypeVolume, Target: testComposeVolumeTarget}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := parseComposeVolume(tt.spec, t.TempDir(), nil)
			assertErrorContains(t, err, "volume requires source and target")
		})
	}
}

func TestLongVolumeMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec ComposeVolume
		want string
	}{
		{name: "default read write", want: volumeModeReadWrite},
		{name: "read only", spec: ComposeVolume{ReadOnly: true}, want: volumeModeReadOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := longVolumeMode(tt.spec); got != tt.want {
				t.Fatalf("longVolumeMode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLongVolumeSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec ComposeVolume
		want string
	}{
		{name: "read write", spec: ComposeVolume{Source: testComposeVolumeName, Target: testComposeVolumeTarget}, want: "data:/var/lib/data:rw"},
		{name: "read only", spec: ComposeVolume{Source: testComposeBindSource, Target: testComposeBindTarget, ReadOnly: true}, want: "./data:/mnt/data:ro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := longVolumeSpec(tt.spec); got != tt.want {
				t.Fatalf("longVolumeSpec = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseVolumeSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
	}{
		{"", defaultVolumeSizeBytes},
		{"  2gb  ", 2 * (1 << 30)},
		{"10G", 10 * (1 << 30)},
		{"2GB", 2 * (1 << 30)},
		{"500M", 500 * (1 << 20)},
		{"512MB", 512 * (1 << 20)},
		{"1T", 1 << 40},
		{"2048K", 2048 << 10},
		{"1048576", 1 << 20},
	}
	for _, tc := range cases {
		got, err := parseVolumeSize(tc.in)
		if err != nil {
			t.Fatalf("parseVolumeSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parseVolumeSize(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}

	if _, err := parseVolumeSize("bogus"); err == nil {
		t.Fatal("expected error on bogus size")
	}
	if _, err := parseVolumeSize("100"); err == nil {
		t.Fatal("expected error on size below minimum")
	}
}

func TestNormalizeVolumeSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "blank", raw: " \t ", want: ""},
		{name: "uppercases and trims", raw: "  2gb  ", want: "2G"},
		{name: "keeps plain bytes", raw: "1048576", want: "1048576"},
		{name: "keeps bare unit", raw: "500m", want: "500M"},
		{name: "rejects bare bytes suffix", raw: "B", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeVolumeSize(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeVolumeSize(%q) err = %v, wantErr %v", tt.raw, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeVolumeSize(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestVolumeSizeMultiplier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		unit byte
		want int64
	}{
		{unit: volumeSizeUnitKiB, want: 1 << 10},
		{unit: volumeSizeUnitMiB, want: 1 << 20},
		{unit: volumeSizeUnitGiB, want: 1 << 30},
		{unit: volumeSizeUnitTiB, want: 1 << 40},
		{unit: '5', want: 1},
	}
	for _, tt := range tests {
		t.Run(string(tt.unit), func(t *testing.T) {
			t.Parallel()

			if got := volumeSizeMultiplier(tt.unit); got != tt.want {
				t.Fatalf("volumeSizeMultiplier(%q) = %d, want %d", tt.unit, got, tt.want)
			}
		})
	}
}
