package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

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
