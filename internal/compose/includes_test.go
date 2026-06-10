package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAcceptsComposeIncludeAndModeSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, "labels.env", "com.example.mode=host\n")
	yamlDoc := `
name: includecompat
include:
  - path:
      - ../commons/compose.yaml
      - ./override.yaml
    project_directory: ..
    env_file:
      - ../commons/.env
  - ./simple.yaml
services:
  vm:
    image: ./base.qcow2
    label_file:
      - ./labels.env
    network_mode: host
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestLoadMergesExistingComposeIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	included := `
services:
  db:
    image: ./base.qcow2
volumes:
  data:
    size: 3G
`
	writeTestFile(t, dir, "common.yaml", included)
	main := `
name: includemerge
include:
  - ./common.yaml
services:
  api:
    image: ./base.qcow2
    depends_on: [db]
    volumes:
      - data:/data
`
	project := resolveTestCompose(t, dir, main)
	if _, ok := project.Services["db"]; !ok {
		t.Fatalf("included service db missing: %#v", project.Services)
	}
	if got := project.Volumes["data"].SizeBytes; got != 3*(1<<30) {
		t.Fatalf("included volume size = %d, want 3G", got)
	}
}

func TestLoadIncludeProjectDirectoryResolvesServicePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commonDir := filepath.Join(dir, "common")
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatalf("mkdir common: %v", err)
	}
	imagePath := filepath.Join(commonDir, "base.qcow2")
	writeTestFile(t, commonDir, "base.qcow2", "not really qcow2")
	included := `
services:
  db:
    image: ./base.qcow2
`
	writeTestFile(t, commonDir, "compose.yaml", included)
	main := `
name: includeprojectdir
include:
  - path: compose.yaml
    project_directory: ./common
services:
  api:
    image: ./common/base.qcow2
    depends_on: [db]
`
	project := resolveTestCompose(t, dir, main)
	if got := project.Services["db"].Image; got != imagePath {
		t.Fatalf("included service image = %q, want %q", got, imagePath)
	}
}

func TestIncludeProjectDir(t *testing.T) {
	t.Parallel()

	baseDir := filepath.Join(t.TempDir(), "project")
	absDir := filepath.Join(t.TempDir(), "shared")

	tests := []struct {
		name    string
		include IncludeFile
		want    string
	}{
		{name: "default", want: baseDir},
		{name: "relative", include: IncludeFile{ProjectDirectory: "shared"}, want: filepath.Join(baseDir, "shared")},
		{name: "absolute", include: IncludeFile{ProjectDirectory: absDir}, want: absDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := includeProjectDir(baseDir, tt.include); got != tt.want {
				t.Fatalf("includeProjectDir = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveIncludePath(t *testing.T) {
	t.Parallel()

	projectDir := filepath.Join(t.TempDir(), "project")
	absPath := filepath.Join(t.TempDir(), "compose.yaml")

	tests := []struct {
		name        string
		includePath string
		want        string
	}{
		{name: "relative", includePath: "compose.yaml", want: filepath.Join(projectDir, "compose.yaml")},
		{name: "absolute", includePath: absPath, want: absPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveIncludePath(projectDir, tt.includePath); got != tt.want {
				t.Fatalf("resolveIncludePath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIncludeFileExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "compose.yaml", "services: {}\n")

	exists, err := includeFileExists(path)
	if err != nil {
		t.Fatalf("includeFileExists existing: %v", err)
	}
	if !exists {
		t.Fatal("includeFileExists existing = false, want true")
	}

	exists, err = includeFileExists(filepath.Join(dir, "missing.yaml"))
	if err != nil {
		t.Fatalf("includeFileExists missing: %v", err)
	}
	if exists {
		t.Fatal("includeFileExists missing = true, want false")
	}
}

func TestAttributeIncludedServiceBaseDirsFillsMissingEntries(t *testing.T) {
	t.Parallel()

	projectDir := filepath.Join(t.TempDir(), "included")
	originalDir := filepath.Join(t.TempDir(), "nested")
	file := &File{
		Services: map[string]Service{
			"api": {},
			"db":  {},
		},
		serviceBaseDirs: map[string]string{
			"db": originalDir,
		},
	}

	attributeIncludedServiceBaseDirs(file, projectDir)

	if got := file.serviceBaseDirs["api"]; got != projectDir {
		t.Fatalf("api service base dir = %q, want %q", got, projectDir)
	}
	if got := file.serviceBaseDirs["db"]; got != originalDir {
		t.Fatalf("db service base dir = %q, want existing %q", got, originalDir)
	}
}

func TestMergeIncludedFilePreservesDestinationPrecedence(t *testing.T) {
	t.Parallel()

	dstDir := filepath.Join(t.TempDir(), "dst")
	srcDir := filepath.Join(t.TempDir(), "src")
	dst := &File{
		Services: map[string]Service{
			"api": {Image: "dst.qcow2"},
		},
		serviceBaseDirs: map[string]string{
			"api": dstDir,
		},
	}
	src := &File{
		Services: map[string]Service{
			"api": {Image: "src.qcow2"},
			"db":  {Image: "db.qcow2"},
		},
		serviceBaseDirs: map[string]string{
			"api": srcDir,
			"db":  srcDir,
		},
	}

	mergeIncludedFile(dst, src)

	if got := dst.Services["api"].Image; got != "dst.qcow2" {
		t.Fatalf("api image = %q, want destination value", got)
	}
	if got := dst.Services["db"].Image; got != "db.qcow2" {
		t.Fatalf("db image = %q, want source value", got)
	}
	if got := dst.serviceBaseDirs["api"]; got != dstDir {
		t.Fatalf("api service base dir = %q, want destination %q", got, dstDir)
	}
	if got := dst.serviceBaseDirs["db"]; got != srcDir {
		t.Fatalf("db service base dir = %q, want source %q", got, srcDir)
	}
}

func TestFindFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	secondaryPath := writeTestFile(t, dir, defaultComposeFileSecondary, "name: secondary\nservices:\n  x:\n    image: a.qcow2\n")

	found, err := FindFile(dir)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found != secondaryPath {
		t.Fatalf("expected %s, got %s", secondaryPath, found)
	}

	primaryPath := writeTestFile(t, dir, defaultComposeFilePrimary, "name: primary\nservices:\n  x:\n    image: a.qcow2\n")
	found, err = FindFile(dir)
	if err != nil {
		t.Fatalf("find after primary added: %v", err)
	}
	if found != primaryPath {
		t.Fatalf("expected %s, got %s", primaryPath, found)
	}
}

func TestFindFileReportsMissingDefault(t *testing.T) {
	t.Parallel()

	_, err := FindFile(t.TempDir())
	assertErrorContains(t, err, "no "+defaultComposeFilePrimary+" found")
}

func TestDefaultFilesReturnsPriorityOrder(t *testing.T) {
	t.Parallel()

	got := DefaultFiles()
	want := []string{defaultComposeFilePrimary, defaultComposeFileSecondary}
	assertStringSliceEqual(t, "DefaultFiles", got, want)
}

func TestDefaultProjectName(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "project")
	path := filepath.Join(dir, defaultComposeFilePrimary)
	if got := defaultProjectName(path); got != "project" {
		t.Fatalf("defaultProjectName = %q, want project", got)
	}

	relPath := filepath.Join("nested", defaultComposeFileSecondary)
	if got := defaultProjectName(relPath); got != "nested" {
		t.Fatalf("defaultProjectName relative = %q, want nested", got)
	}
}
