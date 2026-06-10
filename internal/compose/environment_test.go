package compose

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	testEnvRackKey      = "RACK_ENV"
	testEnvShowKey      = "SHOW"
	testEnvUserInputKey = "USER_INPUT"
	testEnvExtraKey     = "EXTRA"
	testEnvRawKey       = "RAW"
	testEnvExprKey      = "EXPR"
	testEnvAppKey       = "APP_ENV"
	testEnvBaseFile     = "base.env"
	testEnvOverrideFile = "override.env"
	testEnvRawFile      = "raw.env"
	testEnvVarsFile     = "vars.env"
)

func assertEnvironmentFile(t *testing.T, service string, project *Project, want []string) {
	t.Helper()

	wantLines := make([]string, 0, len(want))
	for _, line := range want {
		wantLines = append(wantLines, line+"\n")
	}
	assertWriteFileContains(t, service, environmentFilePath, project.Services[service].CloudInit.WriteFiles, wantLines...)
}

func TestResolveAcceptsEnvironmentSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: env
services:
  map:
    image: ./base.qcow2
    environment:
      RACK_ENV: development
      SHOW: "true"
      USER_INPUT:
  list:
    image: ./base.qcow2
    environment:
      - RACK_ENV=production
      - SHOW=false
      - USER_INPUT
`
	project := resolveTestCompose(t, dir, yamlDoc)
	assertEnvironmentFile(t, "map", project, []string{`RACK_ENV="development"`, `SHOW="true"`})
	assertEnvironmentFile(t, "list", project, []string{`RACK_ENV="production"`, `SHOW="false"`})
}

func TestParseEnvironmentEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantKey   string
		wantValue *string
	}{
		{name: "assignment", raw: "RACK_ENV=production", wantKey: testEnvRackKey, wantValue: stringPtr("production")},
		{name: "pass through", raw: "USER_INPUT", wantKey: testEnvUserInputKey, wantValue: nil},
		{name: "keeps equals in value", raw: "EXPR=a=b", wantKey: "EXPR", wantValue: stringPtr("a=b")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, value := parseEnvironmentEntry(tt.raw)
			if key != tt.wantKey {
				t.Fatalf("key = %q, want %q", key, tt.wantKey)
			}
			if stringValue(value) != stringValue(tt.wantValue) {
				t.Fatalf("value = %q, want %q", stringValue(value), stringValue(tt.wantValue))
			}
			if (value == nil) != (tt.wantValue == nil) {
				t.Fatalf("value nilness = %v, want %v", value == nil, tt.wantValue == nil)
			}
		})
	}
}

func TestDecodeEnvironmentMapValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    *string
		wantErr string
	}{
		{name: "string", raw: `"production"`, want: stringPtr("production")},
		{name: "null", raw: "null", want: nil},
		{name: "invalid", raw: "{RACK_ENV: production}", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal node: %v", err)
			}
			got, err := decodeEnvironmentMapValue(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeEnvironmentMapValue: %v", err)
			}
			if stringValue(got) != stringValue(tt.want) {
				t.Fatalf("decodeEnvironmentMapValue = %q, want %q", stringValue(got), stringValue(tt.want))
			}
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("decodeEnvironmentMapValue nilness = %v, want %v", got == nil, tt.want == nil)
			}
		})
	}
}

func TestMergeEnvironment(t *testing.T) {
	t.Parallel()

	dst := Environment{
		"BASE":     stringPtr("from-file"),
		"OVERRIDE": stringPtr("old"),
	}
	src := Environment{
		"OVERRIDE": stringPtr("new"),
		"PASS":     nil,
	}

	mergeEnvironment(dst, src)

	if got := stringValue(dst["BASE"]); got != "from-file" {
		t.Fatalf("BASE = %q, want from-file", got)
	}
	if got := stringValue(dst["OVERRIDE"]); got != "new" {
		t.Fatalf("OVERRIDE = %q, want new", got)
	}
	if value, ok := dst["PASS"]; !ok || value != nil {
		t.Fatalf("PASS = %v, %v; want nil value present", value, ok)
	}
}

func TestEnvironmentFileRendersSortedAssignedValues(t *testing.T) {
	t.Parallel()

	wf, ok := environmentFile(Environment{
		testEnvShowKey:      stringPtr("true"),
		testEnvUserInputKey: nil,
		testEnvRackKey:      stringPtr("production"),
	})
	if !ok {
		t.Fatal("environmentFile ok = false, want true")
	}
	if wf.Path != environmentFilePath {
		t.Fatalf("environment file path = %q, want %q", wf.Path, environmentFilePath)
	}
	if got, want := wf.Content, "RACK_ENV=\"production\"\nSHOW=\"true\"\n"; got != want {
		t.Fatalf("environment file content = %q, want %q", got, want)
	}
}

func TestEnvironmentFileSkipsPassThroughOnlyValues(t *testing.T) {
	t.Parallel()

	_, ok := environmentFile(Environment{testEnvUserInputKey: nil})
	if ok {
		t.Fatal("environmentFile ok = true, want false")
	}
}

func TestEnvironmentFileContent(t *testing.T) {
	t.Parallel()

	env := Environment{
		testEnvShowKey: stringPtr("true"),
		testEnvRackKey: stringPtr("production"),
	}
	keys := []string{testEnvRackKey, testEnvShowKey}
	if got, want := environmentFileContent(env, keys), "RACK_ENV=\"production\"\nSHOW=\"true\"\n"; got != want {
		t.Fatalf("environmentFileContent = %q, want %q", got, want)
	}
}

func TestEnvironmentWriteFile(t *testing.T) {
	t.Parallel()

	wf := environmentWriteFile("RACK_ENV=\"production\"\n")
	if wf.Path != environmentFilePath {
		t.Fatalf("Path = %q, want %q", wf.Path, environmentFilePath)
	}
	if wf.Content != "RACK_ENV=\"production\"\n" {
		t.Fatalf("Content = %q", wf.Content)
	}
	if wf.Permissions != "0644" || wf.Owner != "root:root" {
		t.Fatalf("metadata = permissions %q owner %q", wf.Permissions, wf.Owner)
	}
}

func TestEnvironmentFileAssignment(t *testing.T) {
	t.Parallel()

	if got, want := environmentFileAssignment(testEnvRackKey, "production"), "RACK_ENV=\"production\"\n"; got != want {
		t.Fatalf("environmentFileAssignment = %q, want %q", got, want)
	}
}

func TestEnvironmentPrefixRendersSortedShellAssignments(t *testing.T) {
	t.Parallel()

	got := environmentPrefix(Environment{
		testEnvShowKey:      stringPtr("true"),
		testEnvUserInputKey: nil,
		testEnvRackKey:      stringPtr("production mode"),
	})
	want := "RACK_ENV='production mode' SHOW=true"
	if got != want {
		t.Fatalf("environmentPrefix = %q, want %q", got, want)
	}
}

func TestEnvironmentShellAssignment(t *testing.T) {
	t.Parallel()

	if got, want := environmentShellAssignment(testEnvShowKey, "true"), "SHOW=true"; got != want {
		t.Fatalf("environmentShellAssignment plain = %q, want %q", got, want)
	}
	if got, want := environmentShellAssignment(testEnvRackKey, "production mode"), "RACK_ENV='production mode'"; got != want {
		t.Fatalf("environmentShellAssignment quoted = %q, want %q", got, want)
	}
}

func TestAssignedEnvironmentKeys(t *testing.T) {
	t.Parallel()

	got := assignedEnvironmentKeys(Environment{
		testEnvShowKey:      stringPtr("true"),
		testEnvUserInputKey: nil,
		testEnvRackKey:      stringPtr("production"),
	})
	want := []string{testEnvRackKey, testEnvShowKey}
	assertStringSliceEqual(t, "assignedEnvironmentKeys", got, want)
}

func TestResolveAcceptsEnvFileSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, testEnvBaseFile, "RACK_ENV=from-file\nSHOW=false\n")
	writeTestFile(t, dir, testEnvOverrideFile, "SHOW=true\nEXTRA=1\n")
	writeTestFile(t, dir, testEnvRawFile, "RAW=$NOT_EXPANDED\n")
	yamlDoc := `
name: envfile
services:
  api:
    image: ./base.qcow2
    env_file:
      - ./base.env
      - path: ./override.env
        required: true
      - path: ./missing.env
        required: "false"
      - path: ./raw.env
        format: raw
    environment:
      RACK_ENV: inline
`
	project := resolveTestCompose(t, dir, yamlDoc)
	assertEnvironmentFile(t, testComposeAPIService, project, []string{
		testEnvRackKey + `="inline"`,
		testEnvShowKey + `="true"`,
		testEnvExtraKey + `="1"`,
		testEnvRawKey + `="$NOT_EXPANDED"`,
	})
}

func TestResolveRejectsRequiredMissingEnvFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: missingenv
services:
  api:
    image: ./base.qcow2
    env_file:
      - ./missing.env
`
	file := loadTestCompose(t, dir, yamlDoc)
	_, err := file.Resolve(dir, dir)
	assertErrorContains(t, err, `env_file "./missing.env"`)
}

func TestResolveRejectsUnsupportedEnvFileFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, testEnvVarsFile, "A=1\n")
	yamlDoc := `
name: badenvformat
services:
  api:
    image: ./base.qcow2
    env_file:
      - path: ./vars.env
        format: shell
`
	file := loadTestCompose(t, dir, yamlDoc)
	_, err := file.Resolve(dir, dir)
	assertErrorContains(t, err, `env_file format "shell" is unsupported`)
}

func TestReadEnvFileParsesCommentsAssignmentsAndPassThrough(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, testEnvVarsFile)
	writeTestFile(t, dir, testEnvVarsFile, `
# ignored
RACK_ENV=production
SHOW = true
USER_INPUT
`)

	env, err := readEnvFile(path)
	if err != nil {
		t.Fatalf("readEnvFile: %v", err)
	}
	if got := stringValue(env[testEnvRackKey]); got != "production" {
		t.Fatalf("RACK_ENV = %q, want production", got)
	}
	if got := stringValue(env[testEnvShowKey]); got != "true" {
		t.Fatalf("SHOW = %q, want true", got)
	}
	if _, ok := env[testEnvUserInputKey]; !ok {
		t.Fatalf("USER_INPUT missing from env: %#v", env)
	}
	if env[testEnvUserInputKey] != nil {
		t.Fatalf("USER_INPUT = %q, want nil pass-through value", *env[testEnvUserInputKey])
	}
}

func TestParseEnvFileLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lineNo  int
		line    string
		wantKey string
		wantVal *string
		wantOK  bool
		wantErr string
	}{
		{name: "blank", lineNo: 1, line: "  ", wantOK: false},
		{name: "comment", lineNo: 2, line: " # ignored", wantOK: false},
		{name: "assignment", lineNo: 3, line: "SHOW = true", wantKey: testEnvShowKey, wantVal: stringPtr("true"), wantOK: true},
		{name: "pass through", lineNo: 4, line: "USER_INPUT", wantKey: testEnvUserInputKey, wantOK: true},
		{name: "keeps equals in value", lineNo: 5, line: "EXPR=a=b", wantKey: testEnvExprKey, wantVal: stringPtr("a=b"), wantOK: true},
		{name: "empty name", lineNo: 6, line: "=value", wantErr: "line 6: empty variable name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, value, ok, err := parseEnvFileLine(tt.lineNo, tt.line)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("parseEnvFileLine: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Fatalf("key = %q, want %q", key, tt.wantKey)
			}
			if stringValue(value) != stringValue(tt.wantVal) {
				t.Fatalf("value = %q, want %q", stringValue(value), stringValue(tt.wantVal))
			}
			if (value == nil) != (tt.wantVal == nil) {
				t.Fatalf("value nilness = %v, want %v", value == nil, tt.wantVal == nil)
			}
		})
	}
}

func TestReadEnvFileRejectsEmptyVariableName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, testEnvVarsFile)
	writeTestFile(t, dir, testEnvVarsFile, "=value\n")

	_, err := readEnvFile(path)
	assertErrorContains(t, err, "empty variable name")
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
