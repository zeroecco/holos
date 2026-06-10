package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/images"
)

const (
	testImageAlpinePath = "/var/lib/holos/cache/alpine.qcow2"
	testImageNobleTag   = "noble"
)

func TestRunPullPrintsLocalImageResult(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return runPull([]string{testImageAlpinePath})
	})
	if err != nil {
		t.Fatalf("runPull: %v", err)
	}

	want := "image: /var/lib/holos/cache/alpine.qcow2\n" +
		"format: qcow2\n"
	if got := out; got != want {
		t.Fatalf("runPull stdout = %q, want %q", got, want)
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	err = fn()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}

	var out bytes.Buffer
	if _, copyErr := io.Copy(&out, reader); copyErr != nil {
		t.Fatalf("read stdout: %v", copyErr)
	}
	return out.String(), err
}

func TestImageCommandArgumentErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "pull missing image", got: pullMissingImageErrorMsg, want: "pull requires an image name (e.g. alpine, ubuntu:noble)"},
		{name: "verify all with args", got: verifyAllWithArgsErrorMsg, want: "verify --all does not accept image arguments"},
		{name: "verify missing target", got: verifyMissingTargetErrorMsg, want: "verify requires an image name, local path, or --all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("error message = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestImageLockOutputPath(t *testing.T) {
	t.Parallel()

	composePath := filepath.Join("/work", "stack", "holos.yaml")
	if got, want := imageLockOutputPath("", composePath), filepath.Join("/work", "stack", defaultImageLockName); got != want {
		t.Fatalf("default lock path = %q, want %q", got, want)
	}
	if got := imageLockOutputPath("/tmp/custom.lock", composePath); got != "/tmp/custom.lock" {
		t.Fatalf("explicit lock path = %q, want explicit", got)
	}
}

func TestImageLockfileForProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	apiPath := writeImageLockTestFile(t, dir, "api.qcow2", "api-image")
	webPath := writeImageLockTestFile(t, dir, "web.raw", "web-image")
	project := &compose.Project{
		Name: "demo",
		Services: map[string]config.Manifest{
			"web": {Image: webPath, ImageFormat: config.ImageFormatRaw},
			"api": {Image: apiPath, ImageFormat: config.ImageFormatQCOW2},
		},
	}

	lockfile, err := imageLockfileForProject(project)
	if err != nil {
		t.Fatalf("imageLockfileForProject: %v", err)
	}
	if lockfile.Version != 1 || lockfile.Project != "demo" {
		t.Fatalf("lockfile identity = %+v, want version 1 project demo", lockfile)
	}
	if len(lockfile.Images) != 2 {
		t.Fatalf("lockfile images = %+v, want two", lockfile.Images)
	}
	assertImageLockEntry(t, lockfile.Images[0], imageLockEntry{
		Service:   "api",
		Path:      apiPath,
		Format:    config.ImageFormatQCOW2,
		SizeBytes: int64(len("api-image")),
		SHA256:    testSHA256("api-image"),
	})
	assertImageLockEntry(t, lockfile.Images[1], imageLockEntry{
		Service:   "web",
		Path:      webPath,
		Format:    config.ImageFormatRaw,
		SizeBytes: int64(len("web-image")),
		SHA256:    testSHA256("web-image"),
	})
}

func TestRunImagesLockWritesLockfile(t *testing.T) {
	dir := t.TempDir()
	imagePath := writeImageLockTestFile(t, dir, "base.qcow2", "base-image")
	composePath := filepath.Join(dir, "holos.yaml")
	body := "name: demo\nservices:\n  web:\n    image: ./base.qcow2\n"
	if err := os.WriteFile(composePath, []byte(body), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	outputPath := filepath.Join(dir, "custom.images.lock")

	out, err := captureStdout(t, func() error {
		return runImages([]string{"lock", "-f", composePath, "-o", outputPath, "--state-dir", filepath.Join(dir, "state")})
	})
	if err != nil {
		t.Fatalf("runImages lock: %v", err)
	}
	if !strings.Contains(out, "wrote "+outputPath) {
		t.Fatalf("lock stdout = %q, want output path", out)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	var lockfile imageLockfile
	if err := json.Unmarshal(payload, &lockfile); err != nil {
		t.Fatalf("decode lockfile: %v", err)
	}
	if len(lockfile.Images) != 1 {
		t.Fatalf("lockfile images = %+v, want one", lockfile.Images)
	}
	if lockfile.Images[0].Path != imagePath || lockfile.Images[0].SHA256 != testSHA256("base-image") {
		t.Fatalf("lockfile image = %+v, want path %q digest", lockfile.Images[0], imagePath)
	}
}

func TestFormatVerifyLines(t *testing.T) {
	t.Parallel()

	res := images.Verification{
		Algorithm: "sha256",
		Hash:      "abcdef0123456789",
		Path:      testImageAlpinePath,
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "missing cache",
			got:  formatVerifyMissingCache("alpine"),
			want: "alpine: skipped (not cached)",
		},
		{
			name: "skipped",
			got:  formatVerifySkipped("local.img", "local image has no registry checksum metadata"),
			want: "local.img: skipped (local image has no registry checksum metadata)",
		},
		{
			name: "success",
			got:  formatVerifySuccess("alpine", res),
			want: "alpine: verified sha256:abcdef0123456789 /var/lib/holos/cache/alpine.qcow2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("verify line = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func writeImageLockTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write image %s: %v", name, err)
	}
	return path
}

func testSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func assertImageLockEntry(t *testing.T, got, want imageLockEntry) {
	t.Helper()

	if got != want {
		t.Fatalf("image lock entry = %+v, want %+v", got, want)
	}
}

func TestWriteImagesTable(t *testing.T) {
	t.Parallel()

	available := []images.Image{
		{
			Name:     "alpine",
			Tag:      "latest",
			Format:   "qcow2",
			OSFamily: "openrc",
			Default:  true,
			SHA256:   "abc",
		},
		{
			Name:     "ubuntu",
			Tag:      testImageNobleTag,
			Format:   "raw",
			OSFamily: "systemd",
		},
	}

	var out bytes.Buffer
	if err := writeImagesTable(&out, available); err != nil {
		t.Fatalf("writeImagesTable: %v", err)
	}
	want := "NAME      TAG     FORMAT  OS       VERIFY\n" +
		"alpine *  latest  qcow2   openrc   sha256\n" +
		"ubuntu    noble   raw     systemd  -\n"
	if got := out.String(); got != want {
		t.Fatalf("images table = %q, want %q", got, want)
	}
}

func TestRunVerifyMissingCacheHandling(t *testing.T) {
	originalRegistry := images.Registry
	images.Registry = []images.Image{
		{
			Name:    "unchecked",
			Tag:     testImageNobleTag,
			URL:     "https://example.com/unchecked.qcow2",
			Format:  "qcow2",
			Default: true,
			SHA256:  strings.Repeat("a", 64),
		},
	}
	t.Cleanup(func() { images.Registry = originalRegistry })

	stateDir := t.TempDir()
	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantErr    string
	}{
		{
			name:       "all skips missing cached image",
			args:       []string{"--state-dir", stateDir, "--all"},
			wantOutput: "unchecked:noble: skipped (not cached)\n",
		},
		{
			name:    "single image reports missing cache",
			args:    []string{"--state-dir", stateDir, "unchecked:" + testImageNobleTag},
			wantErr: "verify unchecked:noble: stat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return runVerify(tt.args)
			})
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				if out != "" {
					t.Fatalf("runVerify stdout = %q, want empty", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("runVerify: %v", err)
			}
			if out != tt.wantOutput {
				t.Fatalf("runVerify stdout = %q, want %q", out, tt.wantOutput)
			}
		})
	}
}
