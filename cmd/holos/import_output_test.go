package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/compose"
)

func TestWriteImportOutputWritesStdout(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "default", output: ""},
		{name: "dash", output: importStdoutOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := compose.File{
				Name: "imported",
				Services: map[string]compose.Service{
					"vm": {Image: "debian:13"},
				},
			}

			originalStdout := os.Stdout
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe stdout: %v", err)
			}
			os.Stdout = writer
			defer func() {
				os.Stdout = originalStdout
			}()

			err = writeImportOutput(tt.output, file, nil)
			if closeErr := writer.Close(); closeErr != nil {
				t.Fatalf("close stdout pipe: %v", closeErr)
			}
			if err != nil {
				t.Fatalf("writeImportOutput: %v", err)
			}

			var out bytes.Buffer
			if _, err := io.Copy(&out, reader); err != nil {
				t.Fatalf("read stdout: %v", err)
			}
			assertImportedComposeYAML(t, out.String())
		})
	}
}

func TestWriteImportOutputWritesComposeFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "compose.yaml")
	file := compose.File{
		Name: "imported",
		Services: map[string]compose.Service{
			"vm": {Image: "debian:13"},
		},
	}

	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = writer
	defer func() {
		os.Stderr = originalStderr
	}()

	if err := writeImportOutput(output, file, nil); err != nil {
		t.Fatalf("writeImportOutput: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	var stderr bytes.Buffer
	if _, err := io.Copy(&stderr, reader); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	wantSummary := "wrote " + output + " (1 service(s))\n"
	if got := stderr.String(); got != wantSummary {
		t.Fatalf("stderr = %q, want %q", got, wantSummary)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	assertImportedComposeYAML(t, string(data))

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if got := info.Mode().Perm(); got != importOutputPerm {
		t.Fatalf("output mode = %v, want %v", got, importOutputPerm)
	}
}

func TestImportAccumulatorComposeFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		explicit string
		order    []string
		want     string
	}{
		{name: "explicit", explicit: "custom", order: []string{"first"}, want: "custom"},
		{name: "first imported", order: []string{"first", "second"}, want: "first"},
		{name: "fallback", want: "imported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			acc := &importAccumulator{
				file:  compose.File{Services: map[string]compose.Service{}},
				order: tt.order,
			}
			if got := acc.composeFile(tt.explicit).Name; got != tt.want {
				t.Fatalf("composeFile(%q).Name = %q, want %q", tt.explicit, got, tt.want)
			}
		})
	}
}

func TestImportAccumulatorPrefixesWarningsWithServiceName(t *testing.T) {
	t.Parallel()

	acc := newImportAccumulator()
	xml := []byte(`
<domain type='kvm'>
  <name>My Web Server</name>
  <devices>
    <disk type='file' device='disk'>
      <source file='/var/lib/libvirt/images/my-web-server.qcow2'/>
      <target dev='vda' bus='virtio'/>
    </disk>
  </devices>
</domain>
`)

	if err := acc.addDomain("domain.xml", xml); err != nil {
		t.Fatalf("addDomain: %v", err)
	}
	want := `my-web-server: renamed domain "My Web Server" to "my-web-server" to satisfy compose naming rules`
	assertStringSliceEqual(t, "warnings", acc.warnings, []string{want})
}

func assertImportedComposeYAML(t *testing.T, got string) {
	t.Helper()

	for _, want := range []string{"name: imported", "vm:", "image: debian:13"} {
		if !strings.Contains(got, want) {
			t.Fatalf("imported compose YAML missing %q:\n%s", want, got)
		}
	}
}
