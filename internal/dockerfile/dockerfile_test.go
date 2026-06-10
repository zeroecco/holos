package dockerfile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testCopyChmod       = "0755"
	testCopyMode        = "755"
	testCopyOwner       = "app:app"
	testCopyConfigDest  = "/etc/app/config"
	testCopyScriptDest  = "/usr/local/bin/run.sh"
	testDockerWorkdir   = "/opt/app"
	testDockerBaseImage = "ubuntu:noble"
)

func writeDockerfile(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("expected content to contain %q, got:\n%s", want, got)
		}
	}
}

func assertOmits(t *testing.T, got, forbidden string) {
	t.Helper()

	if strings.Contains(got, forbidden) {
		t.Fatalf("expected content to omit %q, got:\n%s", forbidden, got)
	}
}

func assertEnvEqual(t *testing.T, got, want [][2]string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("parseEnv = %v, want %v", got, want)
	}
}

type testWriteFileWant struct {
	path        string
	content     string
	permissions string
	owner       string
}

func assertWriteFile(t *testing.T, name string, got config.WriteFile, want testWriteFileWant) {
	t.Helper()

	if got.Path != want.path ||
		got.Content != want.content ||
		got.Permissions != want.permissions ||
		got.Owner != want.owner {
		t.Fatalf("%s write file = %+v, want %+v", name, got, want)
	}
}

func TestParseBasicDockerfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Source file for COPY
	if err := os.WriteFile(filepath.Join(dir, "app.conf"), []byte("listen 80;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dfContent := `FROM ubuntu:noble

# comment ignored by parser
ENV APP_PORT=8080
ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /opt/app

RUN apt-get update && \
    apt-get install -y nginx

RUN echo "hello"

COPY app.conf /etc/nginx/conf.d/
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.FromImage != testDockerBaseImage {
		t.Errorf("FromImage = %q, want %s", result.FromImage, testDockerBaseImage)
	}
	if !strings.HasPrefix(result.Script, buildScriptPrelude) {
		t.Fatalf("script prefix = %q, want %q", result.Script, buildScriptPrelude)
	}

	assertContains(t, result.Script,
		"export APP_PORT=8080",
		"export DEBIAN_FRONTEND=noninteractive",
		"mkdir -p /opt/app && cd /opt/app",
		"apt-get update",
		"apt-get install -y nginx",
		`echo "hello"`,
	)
	assertOmits(t, result.Script, "comment ignored by parser")

	// COPY'd file + build script
	if len(result.WriteFiles) != 2 {
		t.Fatalf("WriteFiles count = %d, want 2", len(result.WriteFiles))
	}

	assertWriteFile(t, "COPY", result.WriteFiles[0], testWriteFileWant{
		path:        "/etc/nginx/conf.d/app.conf",
		content:     "listen 80;\n",
		permissions: defaultCopyPermissions,
		owner:       defaultCopyOwner,
	})
	assertWriteFile(t, "build script", result.WriteFiles[1], testWriteFileWant{
		path:        buildScriptPath,
		content:     result.Script,
		permissions: buildScriptPermissions,
		owner:       buildScriptOwner,
	})
}

func TestBuildScriptWriteFile(t *testing.T) {
	t.Parallel()

	const script = "echo ready\n"
	wf := buildScriptWriteFile(script)

	assertWriteFile(t, "build script", wf, testWriteFileWant{
		path:        buildScriptPath,
		content:     script,
		permissions: buildScriptPermissions,
		owner:       buildScriptOwner,
	})
}

func TestParseWorkdirTrimsAndQuotesPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfPath := writeDockerfile(t, dir, "FROM ubuntu:noble\nWORKDIR  /opt/my app \n")

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	want := "mkdir -p '/opt/my app' && cd '/opt/my app'\n"
	assertContains(t, result.Script, want)
}

func TestParseRejectsUnsupportedInstructions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		instruction string
		dockerfile  string
		wantHint    string
	}{
		{
			instruction: "EXPOSE",
			dockerfile:  "FROM alpine:3.21\nEXPOSE ignored\n",
			wantHint:    "publish ports in holos.yaml",
		},
		{
			instruction: "CMD",
			dockerfile:  "FROM alpine:3.21\nCMD [\"echo\", \"hi\"]\n",
			wantHint:    "cloud_init.runcmd",
		},
		{
			instruction: "ENTRYPOINT",
			dockerfile:  "FROM alpine:3.21\nENTRYPOINT [\"echo\", \"hi\"]\n",
			wantHint:    "cloud_init.runcmd",
		},
		{
			instruction: "ADD",
			dockerfile:  "FROM alpine:3.21\nADD ignored\n",
			wantHint:    "use COPY",
		},
		{
			instruction: "USER",
			dockerfile:  "FROM alpine:3.21\nUSER ignored\n",
			wantHint:    "not supported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.instruction, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dfPath := writeDockerfile(t, dir, tc.dockerfile)

			_, err := Parse(dfPath, dir)
			if err == nil {
				t.Fatalf("expected %s to be rejected", tc.instruction)
			}
			assertContains(t, err.Error(), tc.instruction)
			assertContains(t, err.Error(), tc.wantHint)
		})
	}
}

func TestParseHealthcheckShellForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfPath := writeDockerfile(t, dir, `FROM alpine:3.21
HEALTHCHECK --interval=5s --timeout=2s --start-period=10s --start-interval=1s --retries=4 CMD curl -f http://localhost/health || exit 1
`)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	hc := result.Healthcheck
	if hc == nil {
		t.Fatal("Healthcheck = nil, want parsed config")
	}
	wantTest := []string{healthcheckShellCommand, healthcheckShellFlag, "curl -f http://localhost/health || exit 1"}
	if !slices.Equal(hc.Test, wantTest) {
		t.Fatalf("Healthcheck.Test = %v, want %v", hc.Test, wantTest)
	}
	if hc.IntervalSec != 5 || hc.TimeoutSec != 2 || hc.StartPeriodSec != 10 || hc.StartIntervalSec != 1 || hc.Retries != 4 {
		t.Fatalf("Healthcheck timing = %+v, want interval=5 timeout=2 start_period=10 start_interval=1 retries=4", hc)
	}
}

func TestParseHealthcheckExecForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfPath := writeDockerfile(t, dir, `FROM alpine:3.21
HEALTHCHECK --interval=7s CMD ["curl", "-f", "http://localhost/health"]
`)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	hc := result.Healthcheck
	if hc == nil {
		t.Fatal("Healthcheck = nil, want parsed config")
	}
	wantTest := []string{"curl", "-f", "http://localhost/health"}
	if !slices.Equal(hc.Test, wantTest) {
		t.Fatalf("Healthcheck.Test = %v, want %v", hc.Test, wantTest)
	}
	if hc.IntervalSec != 7 || hc.StartIntervalSec != 7 {
		t.Fatalf("Healthcheck interval = %+v, want interval and start_interval 7", hc)
	}
}

func TestParseHealthcheckNoneClearsPreviousHealthcheck(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfPath := writeDockerfile(t, dir, `FROM alpine:3.21
HEALTHCHECK CMD curl -f http://localhost/health
HEALTHCHECK NONE
`)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if result.Healthcheck != nil {
		t.Fatalf("Healthcheck = %+v, want nil after NONE", result.Healthcheck)
	}
}

func TestParseHealthcheckRejectsInvalidForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dockerfile string
		want       string
	}{
		{name: "missing command", dockerfile: "FROM alpine\nHEALTHCHECK CMD\n", want: "requires a command"},
		{name: "missing cmd", dockerfile: "FROM alpine\nHEALTHCHECK curl -f http://localhost\n", want: "requires CMD or NONE"},
		{name: "unsupported option", dockerfile: "FROM alpine\nHEALTHCHECK --bogus=1 CMD true\n", want: "unsupported HEALTHCHECK option"},
		{name: "bad duration", dockerfile: "FROM alpine\nHEALTHCHECK --interval=soon CMD true\n", want: "--interval"},
		{name: "bad retries", dockerfile: "FROM alpine\nHEALTHCHECK --retries=0 CMD true\n", want: "--retries must be >= 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			dfPath := writeDockerfile(t, dir, tt.dockerfile)
			_, err := Parse(dfPath, dir)
			if err == nil {
				t.Fatalf("Parse error = nil, want %q", tt.want)
			}
			assertContains(t, err.Error(), tt.want)
		})
	}
}

func TestUnsupportedInstructionErrorListsSupportedInstructions(t *testing.T) {
	t.Parallel()

	err := unsupportedInstructionError("BOGUS")
	if err == nil {
		t.Fatal("unsupportedInstructionError returned nil")
	}
	assertContains(t, err.Error(), "BOGUS")
	assertContains(t, err.Error(), supportedInstructionList())
}

func TestParseFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args string
		want string
	}{
		{args: "alpine:3.21", want: "alpine:3.21"},
		{args: "--platform=linux/amd64 ubuntu:noble", want: "ubuntu:noble"},
		{args: "--platform=$BUILDPLATFORM golang:1.24 AS builder", want: "golang:1.24"},
		{args: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			t.Parallel()

			if got := parseFrom(tt.args); got != tt.want {
				t.Fatalf("parseFrom(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseExecFormRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfContent := `FROM alpine
RUN ["apk", "add", "curl"]
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	assertContains(t, result.Script, "apk add curl")
}

// TestParseExecFormRunPreservesArgvBoundaries pins the Docker exec-
// form contract: argv elements must survive the trip through our
// cloud-init bash script without being re-split by the shell. Before
// the fix, RUN ["echo", "hello world", "$PATH"] got flattened to
// `echo hello world $PATH`, which bash would tokenize into four
// arguments and expand $PATH. After the fix, each element is
// single-quoted so the guest sees exactly three argv entries and the
// literal string "$PATH".
func TestParseExecFormRunPreservesArgvBoundaries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfContent := `FROM alpine
RUN ["echo", "hello world", "$PATH", "a'b"]
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	assertContains(t, result.Script,
		"'hello world'",
		"'$PATH'",
		`'a'\''b'`,
	)
	assertOmits(t, result.Script, "echo hello world $PATH")
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: "''"},
		{value: "abcXYZ123_-./:", want: "abcXYZ123_-./:"},
		{value: "hello world", want: "'hello world'"},
		{value: "$PATH", want: "'$PATH'"},
		{value: "a'b", want: `'a'\''b'`},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			if got := shellQuote(tt.value); got != tt.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseCopyChmod(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/bash\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dfContent := `FROM ubuntu:noble
COPY --chmod=755 run.sh /usr/local/bin/
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// COPY file + build script
	if len(result.WriteFiles) < 1 {
		t.Fatal("no write_files from COPY")
	}
	wf := result.WriteFiles[0]
	if wf.Permissions != testCopyMode {
		t.Errorf("permissions = %q, want %s", wf.Permissions, testCopyMode)
	}
	if wf.Path != testCopyScriptDest {
		t.Errorf("path = %q, want %s", wf.Path, testCopyScriptDest)
	}
}

func TestParseCopyChown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dfContent := `FROM ubuntu:noble
COPY --chown=app:app config /etc/app/config
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.WriteFiles) < 1 {
		t.Fatal("no write_files from COPY")
	}
	wf := result.WriteFiles[0]
	if wf.Owner != testCopyOwner {
		t.Errorf("owner = %q, want %s", wf.Owner, testCopyOwner)
	}
}

func TestCopySourceEscapesContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rel  string
		want bool
	}{
		{rel: "..", want: true},
		{rel: "../secret", want: true},
		{rel: "..data", want: false},
		{rel: "...", want: false},
		{rel: "nested/..data", want: false},
		{rel: "config/app.conf", want: false},
		{rel: ".", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()

			if got := copySourceEscapesContext(tt.rel); got != tt.want {
				t.Fatalf("copySourceEscapesContext(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestParseEnvLegacyForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfContent := `FROM ubuntu:noble
ENV MY_VAR some value with spaces
`
	dfPath := writeDockerfile(t, dir, dfContent)

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	assertContains(t, result.Script, "export MY_VAR='some value with spaces'")
}

func TestParseEnv(t *testing.T) {
	t.Parallel()

	got := parseEnv(`APP_PORT=8080 MESSAGE="hello world" EMPTY=''`)
	want := [][2]string{
		{"APP_PORT", "8080"},
		{"MESSAGE", "hello world"},
		{"EMPTY", ""},
	}
	assertEnvEqual(t, got, want)
}

func TestParseEnvLegacy(t *testing.T) {
	t.Parallel()

	got := parseEnv("MY_VAR some value with spaces")
	assertEnvEqual(t, got, [][2]string{{"MY_VAR", "some value with spaces"}})
}

func TestSplitInstruction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line     string
		wantCmd  string
		wantArgs string
	}{
		{line: "run echo hello", wantCmd: "RUN", wantArgs: "echo hello"},
		{line: "FROM   alpine:3.21", wantCmd: "FROM", wantArgs: "alpine:3.21"},
		{line: "WORKDIR", wantCmd: "WORKDIR"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()

			gotCmd, gotArgs := splitInstruction(tt.line)
			if gotCmd != tt.wantCmd || gotArgs != tt.wantArgs {
				t.Fatalf("splitInstruction(%q) = (%q, %q), want (%q, %q)", tt.line, gotCmd, gotArgs, tt.wantCmd, tt.wantArgs)
			}
		})
	}
}

// TestCopyRejectsEscapingSource pins the contract that COPY sources
// must stay inside the build context. Without this, a Dockerfile in a
// repo could exfiltrate host files (ssh keys, /etc/shadow, ...) into
// the generated cloud-init write_files, which holos then hands to the
// VM verbatim. Each case plants a genuine secret file on disk and
// asserts that Parse refuses to read it.
func TestCopyRejectsEscapingSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	secret := filepath.Join(root, "secret")
	if err := os.WriteFile(secret, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}

	contextDir := filepath.Join(root, "ctx")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		dockerfile string
	}{
		{
			name: "relative parent traversal",
			dockerfile: `FROM alpine:3.21
COPY ../secret /opt/exfil
`,
		},
		{
			name: "absolute host path",
			dockerfile: fmt.Sprintf(`FROM alpine:3.21
COPY %s /opt/exfil
`, secret),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dfPath := filepath.Join(contextDir, "Dockerfile."+strings.ReplaceAll(tc.name, " ", "_"))
			if err := os.WriteFile(dfPath, []byte(tc.dockerfile), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(dfPath, contextDir); err == nil {
				t.Fatal("expected COPY-escape error, got nil")
			} else {
				assertContains(t, err.Error(), "escapes build context")
			}
		})
	}
}

// TestCopyRejectsSymlinkEscape closes the subtler hole where a
// textually-inside source resolves via symlink to a host file outside
// the context. filepath.EvalSymlinks should catch this before we read
// the target, so the secret never enters a WriteFile.
func TestCopyRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	secret := filepath.Join(root, "shadow")
	if err := os.WriteFile(secret, []byte("root:x:*:0:0:::"), 0o600); err != nil {
		t.Fatal(err)
	}

	contextDir := filepath.Join(root, "ctx")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(contextDir, "inside.link")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	dfContent := `FROM alpine:3.21
COPY inside.link /opt/exfil
`
	dfPath := writeDockerfile(t, contextDir, dfContent)

	if _, err := Parse(dfPath, contextDir); err == nil {
		t.Fatal("expected symlink-escape error, got nil")
	} else {
		assertContains(t, err.Error(), "escapes build context")
	}
}

// TestCopyRejectsMultiSource guards against the silent-dropped-source
// bug: `COPY a b /dst/` previously copied only `a` and let `b`
// disappear without a warning. Users then shipped guests missing the
// files they thought they had. The parser now rejects the form so the
// operator sees the problem immediately.
func TestCopyRejectsMultiSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dfPath := writeDockerfile(t, dir, "FROM alpine:3.21\nCOPY a.txt b.txt /opt/\n")

	_, err := Parse(dfPath, dir)
	if err == nil {
		t.Fatal("expected multi-source COPY to be rejected")
	}
	assertContains(t, err.Error(), "multi-source")
}
