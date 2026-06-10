package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestLogTailLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  string
		lines int
		want  []string
	}{
		{name: "last lines", data: "one\ntwo\nthree\n", lines: 2, want: []string{"two", "three"}},
		{name: "more than available", data: "one\ntwo\n", lines: 5, want: []string{"one", "two"}},
		{name: "no trailing newline", data: "one\ntwo", lines: 1, want: []string{"two"}},
		{name: "zero lines", data: "one\ntwo\n", lines: 0},
		{name: "negative lines", data: "one\ntwo\n", lines: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := logTailLines([]byte(tt.data), tt.lines)
			assertStringSliceEqual(t, "logTailLines", got, tt.want)
		})
	}
}

func TestWriteLogTail(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "console.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var out bytes.Buffer
	var warnings bytes.Buffer
	writeLogTail(&out, &warnings, path, 2)
	if got, want := out.String(), "two\nthree\n"; got != want {
		t.Fatalf("log tail output = %q, want %q", got, want)
	}
	if got := warnings.String(); got != "" {
		t.Fatalf("log tail warnings = %q, want empty", got)
	}
}

func TestWriteLogTailReadError(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var warnings bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.log")
	writeLogTail(&out, &warnings, missing, 2)
	if got := out.String(); got != "" {
		t.Fatalf("log tail output = %q, want empty", got)
	}
	gotWarnings := warnings.String()
	assertContains(t, gotWarnings, "cannot read log")
	assertContains(t, gotWarnings, missing)
}

func TestWriteLogTarget(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "console.log")
	if err := os.WriteFile(path, []byte("boot\nready\nlogin\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	inst := runtime.InstanceRecord{Name: "vm-0", LogPath: path}

	var out bytes.Buffer
	var warnings bytes.Buffer
	writeLogTarget(&out, &warnings, inst, 2)
	want := "==> vm-0 <==\nready\nlogin\n\n"
	if got := out.String(); got != want {
		t.Fatalf("log target output = %q, want %q", got, want)
	}
	if got := warnings.String(); got != "" {
		t.Fatalf("log target warnings = %q, want empty", got)
	}
}
