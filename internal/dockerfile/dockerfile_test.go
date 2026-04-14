package dockerfile

import (
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
