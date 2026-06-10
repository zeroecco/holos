package compose

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnvFilesUnmarshalLongFormFields(t *testing.T) {
	t.Parallel()

	var files EnvFiles
	err := yaml.Unmarshal([]byte(`
- path: ./app.env
  required: "false"
  format: raw
`), &files)
	if err != nil {
		t.Fatalf("unmarshal env_files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	file := files[0]
	if file.Path != "./app.env" {
		t.Fatalf("Path = %q, want ./app.env", file.Path)
	}
	if file.Format != "raw" {
		t.Fatalf("Format = %q, want raw", file.Format)
	}
	if file.required() {
		t.Fatalf("required() = true, want false")
	}
}

func TestEnvFilesRejectsUnknownLongFormField(t *testing.T) {
	t.Parallel()

	var files EnvFiles
	err := yaml.Unmarshal([]byte(`
- path: ./app.env
  unknown: true
`), &files)
	assertErrorContains(t, err, `field unknown not found in type compose.EnvFile`)
}

func TestResolveEnvFilePath(t *testing.T) {
	t.Parallel()

	baseDir := filepath.Join(t.TempDir(), "project")
	absPath := filepath.Join(t.TempDir(), "app.env")

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "relative", path: "app.env", want: filepath.Join(baseDir, "app.env")},
		{name: "absolute", path: absPath, want: absPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveEnvFilePath(baseDir, tt.path); got != tt.want {
				t.Fatalf("resolveEnvFilePath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateEnvFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    EnvFile
		wantErr string
	}{
		{name: "path only", file: EnvFile{Path: "./app.env"}},
		{name: "raw format", file: EnvFile{Path: "./app.env", Format: envFileRawFormat}},
		{name: "missing path", file: EnvFile{}, wantErr: "path is required"},
		{name: "unsupported format", file: EnvFile{Path: "./app.env", Format: "shell"}, wantErr: "format \"shell\" is unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateEnvFile(tt.file)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("validateEnvFile: %v", err)
			}
		})
	}
}

func TestEnvFileRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		required any
		want     bool
	}{
		{name: "omitted", want: true},
		{name: "bool false", required: false, want: false},
		{name: "bool true", required: true, want: true},
		{name: "string false", required: "false", want: false},
		{name: "invalid string defaults true", required: "sometimes", want: true},
		{name: "unsupported type defaults true", required: 0, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := (EnvFile{Required: tt.required}).required(); got != tt.want {
				t.Fatalf("required() = %v, want %v", got, tt.want)
			}
		})
	}
}
