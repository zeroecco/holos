package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestISOBuilderArgs(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join("/tmp", "seed.iso")
	got := isoBuilderArgs(outputPath)
	want := []string{
		"-output", outputPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
	}
	assertStringSliceEqual(t, "isoBuilderArgs", got, want)
}

func TestXorrisoISOArgs(t *testing.T) {
	t.Parallel()

	outputPath := filepath.Join("/tmp", "seed.iso")
	files := []string{
		filepath.Join("/tmp", "user-data"),
		filepath.Join("/tmp", "meta-data"),
	}
	got := xorrisoISOArgs(outputPath, files)
	want := []string{
		"-as", "mkisofs",
		"-output", outputPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
		files[0], files[1],
	}
	assertStringSliceEqual(t, "xorrisoISOArgs", got, want)
}

func TestCloudLocalDSArgs(t *testing.T) {
	t.Parallel()

	paths := newSeedPaths("/tmp/work")
	tests := []struct {
		name       string
		hasNetwork bool
		want       []string
	}{
		{
			name:       "without network config",
			hasNetwork: false,
			want:       []string{"/tmp/work/seed.img", "/tmp/work/seed/user-data", "/tmp/work/seed/meta-data"},
		},
		{
			name:       "with network config",
			hasNetwork: true,
			want: []string{
				"--network-config", "/tmp/work/seed/network-config",
				"/tmp/work/seed.img", "/tmp/work/seed/user-data", "/tmp/work/seed/meta-data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cloudLocalDSArgs(paths.cloudLocalDSImage, paths, tt.hasNetwork)
			assertStringSliceEqual(t, "cloudLocalDSArgs", got, tt.want)
		})
	}
}

func TestSeedContentFiles(t *testing.T) {
	t.Parallel()

	paths := newSeedPaths("/tmp/work")
	tests := []struct {
		name       string
		hasNetwork bool
		want       []string
	}{
		{
			name:       "without network config",
			hasNetwork: false,
			want:       []string{paths.userData, paths.metaData},
		},
		{
			name:       "with network config",
			hasNetwork: true,
			want:       []string{paths.userData, paths.metaData, paths.networkConfig},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := seedContentFiles(paths, tt.hasNetwork)
			assertStringSliceEqual(t, "seedContentFiles", got, tt.want)
		})
	}
}

func TestWriteSeedContent(t *testing.T) {
	t.Parallel()

	paths := newSeedPaths(t.TempDir())
	if err := writeSeedContent(paths, "user-data", "meta-data", "network-config", true); err != nil {
		t.Fatalf("writeSeedContent: %v", err)
	}

	assertMode(t, paths.dir, seedDirPerm)
	assertSeedFile(t, paths.userData, "user-data")
	assertSeedFile(t, paths.metaData, "meta-data")
	assertSeedFile(t, paths.networkConfig, "network-config")
}

func TestWriteSeedContentSkipsEmptyNetworkConfig(t *testing.T) {
	t.Parallel()

	paths := newSeedPaths(t.TempDir())
	if err := writeSeedContent(paths, "user-data", "meta-data", "", false); err != nil {
		t.Fatalf("writeSeedContent: %v", err)
	}

	if _, err := os.Stat(paths.networkConfig); !os.IsNotExist(err) {
		t.Fatalf("network-config stat = %v, want not exist", err)
	}
}

func TestRunSeedBuilderTightensOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, seedISOName)
	builder := writeSeedBuilderScript(t, dir, "success", "printf seed > \"$1\"\n")

	if err := runSeedBuilder("seed iso", outputPath, builder, []string{outputPath}); err != nil {
		t.Fatalf("runSeedBuilder: %v", err)
	}

	assertSeedFile(t, outputPath, "seed")
}

func TestRunSeedBuilderReportsTrimmedOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, seedISOName)
	builder := writeSeedBuilderScript(t, dir, "failure", "printf ' builder failed\\n'; exit 7\n")

	err := runSeedBuilder("seed iso", outputPath, builder, nil)
	assertErrorContains(t, err, "create seed iso")
	assertErrorContains(t, err, "builder failed")
}

func assertSeedFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
	assertMode(t, path, seedFilePerm)
}

func writeSeedBuilderScript(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write seed builder script: %v", err)
	}
	return path
}
