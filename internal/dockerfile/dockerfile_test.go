package dockerfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBasicDockerfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Source file for COPY
	if err := os.WriteFile(filepath.Join(dir, "app.conf"), []byte("listen 80;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dfContent := `FROM ubuntu:noble

ENV APP_PORT=8080
ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /opt/app

RUN apt-get update && \
    apt-get install -y nginx

RUN echo "hello"

COPY app.conf /etc/nginx/conf.d/

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.FromImage != "ubuntu:noble" {
		t.Errorf("FromImage = %q, want ubuntu:noble", result.FromImage)
	}

	if !strings.Contains(result.Script, "export APP_PORT=8080") {
		t.Error("script missing APP_PORT export")
	}
	if !strings.Contains(result.Script, "export DEBIAN_FRONTEND=noninteractive") {
		t.Error("script missing DEBIAN_FRONTEND export")
	}
	if !strings.Contains(result.Script, "mkdir -p /opt/app && cd /opt/app") {
		t.Error("script missing WORKDIR mkdir+cd")
	}
	if !strings.Contains(result.Script, "apt-get update") {
		t.Error("script missing apt-get update")
	}
	if !strings.Contains(result.Script, "apt-get install -y nginx") {
		t.Error("script missing apt-get install")
	}
	if !strings.Contains(result.Script, `echo "hello"`) {
		t.Error("script missing echo command")
	}

	// COPY'd file + build script
	if len(result.WriteFiles) != 2 {
		t.Fatalf("WriteFiles count = %d, want 2", len(result.WriteFiles))
	}

	confFile := result.WriteFiles[0]
	if confFile.Path != "/etc/nginx/conf.d/app.conf" {
		t.Errorf("COPY dest = %q, want /etc/nginx/conf.d/app.conf", confFile.Path)
	}
	if confFile.Content != "listen 80;\n" {
		t.Errorf("COPY content = %q", confFile.Content)
	}

	buildScript := result.WriteFiles[1]
	if buildScript.Path != buildScriptPath {
		t.Errorf("build script path = %q, want %q", buildScript.Path, buildScriptPath)
	}
	if buildScript.Permissions != "0755" {
		t.Errorf("build script perms = %q, want 0755", buildScript.Permissions)
	}
}

func TestParseExecFormRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfContent := `FROM alpine
RUN ["apk", "add", "curl"]
`
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !strings.Contains(result.Script, "apk add curl") {
		t.Errorf("exec form not converted: %s", result.Script)
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
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// COPY file + build script
	if len(result.WriteFiles) < 1 {
		t.Fatal("no write_files from COPY")
	}
	wf := result.WriteFiles[0]
	if wf.Permissions != "755" {
		t.Errorf("permissions = %q, want 755", wf.Permissions)
	}
	if wf.Path != "/usr/local/bin/run.sh" {
		t.Errorf("path = %q, want /usr/local/bin/run.sh", wf.Path)
	}
}

func TestParseEnvLegacyForm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dfContent := `FROM ubuntu:noble
ENV MY_VAR some value with spaces
`
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Parse(dfPath, dir)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !strings.Contains(result.Script, "export MY_VAR='some value with spaces'") {
		t.Errorf("legacy ENV not handled: %s", result.Script)
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

	cases := map[string]string{
		"relative parent traversal": `FROM alpine:3.21
COPY ../secret /opt/exfil
`,
		"absolute host path": fmt.Sprintf(`FROM alpine:3.21
COPY %s /opt/exfil
`, secret),
	}

	for name, dfContent := range cases {
		t.Run(name, func(t *testing.T) {
			dfPath := filepath.Join(contextDir, "Dockerfile."+strings.ReplaceAll(name, " ", "_"))
			if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(dfPath, contextDir); err == nil {
				t.Fatal("expected COPY-escape error, got nil")
			} else if !strings.Contains(err.Error(), "escapes build context") {
				t.Fatalf("error should name the escape; got %v", err)
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
	dfPath := filepath.Join(contextDir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte(dfContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Parse(dfPath, contextDir); err == nil {
		t.Fatal("expected symlink-escape error, got nil")
	} else if !strings.Contains(err.Error(), "escapes build context") {
		t.Fatalf("error should name the escape; got %v", err)
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
	dfPath := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dfPath, []byte("FROM alpine:3.21\nCOPY a.txt b.txt /opt/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Parse(dfPath, dir)
	if err == nil {
		t.Fatal("expected multi-source COPY to be rejected")
	}
	if !strings.Contains(err.Error(), "multi-source") {
		t.Fatalf("error should name the multi-source form; got %v", err)
	}
}
