package compose

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func assertBuildListFieldLen(t *testing.T, name string, gotLen, wantLen int) {
	t.Helper()

	if gotLen != wantLen {
		t.Fatalf("%s len = %d, want %d", name, gotLen, wantLen)
	}
}

func TestResolveAcceptsComposeBuildSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	workerDir := filepath.Join(dir, "worker")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker: %v", err)
	}
	writeTestImage(t, dir)
	writeTestFile(t, appDir, "Dockerfile", "RUN echo api\n")
	writeTestFile(t, workerDir, "Containerfile", "RUN echo worker\n")
	yamlDoc := `
name: buildsyntax
services:
  api:
    image: ./base.qcow2
    build: ./app
  worker:
    image: ./base.qcow2
    build:
      context: ./worker
      dockerfile: Containerfile
      args:
        APP_ENV: production
      additional_contexts:
        - resources=./resources
        - alpine=docker-image://alpine:latest
      cache_from:
        - type=registry,ref=example/cache
      extra_hosts:
        - host.docker.internal=host-gateway
      isolation: default
      labels:
        com.example.role: worker
      no_cache: "true"
      pull: "true"
      provenance: mode=max
      sbom: true
      shm_size: 64M
      ssh:
        default: ~/.ssh/id_ed25519
      tags:
        - example/worker:latest
      target: prod
      ulimits:
        nofile:
          soft: 20000
          hard: 40000
  inline:
    image: ./base.qcow2
    build:
      context: ./app
      dockerfile_inline: |
        RUN echo inline
`
	project := resolveTestCompose(t, dir, yamlDoc)
	for _, service := range []string{"api", "worker", "inline"} {
		assertStringSliceFirst(t, service+" runcmd", project.Services[service].CloudInit.RunCmd, "bash /var/lib/holos/build.sh")
	}
	assertWriteFileContains(t, "inline", "/var/lib/holos/build.sh", project.Services["inline"].CloudInit.WriteFiles, "echo inline")
}

func TestResolveDockerfileBuildAdoptsFromImage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fromDir := filepath.Join(dir, "frombase")
	if err := os.MkdirAll(fromDir, 0o755); err != nil {
		t.Fatalf("mkdir frombase: %v", err)
	}
	writeTestFile(t, fromDir, "Dockerfile", "FROM debian:bookworm\nRUN echo frombase\n")

	file := &File{Name: "buildsyntax"}
	manifest, err := file.resolveService(
		"frombase",
		Service{Build: ComposeBuild{Context: "./frombase"}},
		dir,
		dir,
		file.planNetwork(),
		map[string]string{"frombase": "10.10.0.2", "frombase-0": "10.10.0.2"},
		[]string{"10.10.0.2"},
		composeTestImages,
	)
	if err != nil {
		t.Fatalf("resolveService: %v", err)
	}

	if got, want := manifest.Image, filepath.Join(dir, "debian-bookworm.qcow2"); got != want {
		t.Fatalf("image = %q, want adopted FROM image resolved to %q", got, want)
	}
	assertStringSliceFirst(t, "runcmd", manifest.CloudInit.RunCmd, "bash /var/lib/holos/build.sh")
}

func TestComposeBuildUnmarshalAcceptsKnownFields(t *testing.T) {
	t.Parallel()

	var build ComposeBuild
	err := yaml.Unmarshal([]byte(`
context: ./app
dockerfile: Containerfile
dockerfile_inline: RUN echo inline
args:
  APP_ENV: production
additional_contexts:
  resources: ./resources
cache_from:
  - type=registry,ref=example/cache
cache_to:
  - type=local,dest=.cache
entitlements:
  - network.host
extra_hosts:
  - host.docker.internal=host-gateway
isolation: default
labels:
  com.example.role: worker
network: host
no_cache: true
pull: true
provenance: mode=max
sbom: true
secrets:
  - source: app-secret
shm_size: 64M
ssh:
  default: ~/.ssh/id_ed25519
tags:
  - example/worker:latest
target: prod
ulimits:
  nofile:
    soft: 20000
    hard: 40000
platforms:
  - linux/amd64
privileged: true
`), &build)
	if err != nil {
		t.Fatalf("unmarshal build: %v", err)
	}
	if build.Context != "./app" || build.Dockerfile != "Containerfile" || build.DockerfileInline != "RUN echo inline" {
		t.Fatalf("build paths = context %q dockerfile %q inline %q", build.Context, build.Dockerfile, build.DockerfileInline)
	}
	if build.Target != "prod" || build.Network != "host" || build.Isolation != "default" {
		t.Fatalf("build metadata = target %q network %q isolation %q", build.Target, build.Network, build.Isolation)
	}
	assertBuildListFieldLen(t, "cache_from", len(build.CacheFrom), 1)
	assertBuildListFieldLen(t, "cache_to", len(build.CacheTo), 1)
	assertBuildListFieldLen(t, "entitlements", len(build.Entitlements), 1)
	assertBuildListFieldLen(t, "tags", len(build.Tags), 1)
	assertBuildListFieldLen(t, "platforms", len(build.Platforms), 1)
}

func TestComposeBuildRejectsUnknownField(t *testing.T) {
	t.Parallel()

	var build ComposeBuild
	err := yaml.Unmarshal([]byte("context: ./app\nunexpected: true\n"), &build)
	assertErrorContains(t, err, "field unexpected not found in type compose.ComposeBuild")
}

func TestComposeBuildDockerfilePath(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	appDir := filepath.Join(baseDir, "app")
	customPath := filepath.Join(baseDir, "Containerfile")

	tests := []struct {
		name        string
		build       ComposeBuild
		wantPath    string
		wantContext string
	}{
		{
			name:        "default context and dockerfile",
			build:       ComposeBuild{Context: defaultBuildContextDir},
			wantPath:    filepath.Join(baseDir, defaultBuildContextDir, defaultDockerfileName),
			wantContext: filepath.Join(baseDir, defaultBuildContextDir),
		},
		{
			name:        "custom relative dockerfile",
			build:       ComposeBuild{Context: "app", Dockerfile: "Containerfile"},
			wantPath:    filepath.Join(appDir, "Containerfile"),
			wantContext: appDir,
		},
		{
			name:        "custom absolute dockerfile",
			build:       ComposeBuild{Context: "app", Dockerfile: customPath},
			wantPath:    customPath,
			wantContext: appDir,
		},
		{
			name:        "inline dockerfile has context but no path",
			build:       ComposeBuild{Context: "app", DockerfileInline: "RUN echo inline"},
			wantContext: appDir,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPath, gotContext, ok, err := tt.build.dockerfilePath(baseDir)
			if err != nil {
				t.Fatalf("dockerfilePath: %v", err)
			}
			if !ok {
				t.Fatal("dockerfilePath ok = false, want true")
			}
			if gotPath != tt.wantPath || gotContext != tt.wantContext {
				t.Fatalf("dockerfilePath = (%q, %q), want (%q, %q)", gotPath, gotContext, tt.wantPath, tt.wantContext)
			}
		})
	}
}

func TestComposeBuildDockerfilePathUnset(t *testing.T) {
	t.Parallel()

	path, contextDir, ok, err := (ComposeBuild{}).dockerfilePath(t.TempDir())
	if err != nil {
		t.Fatalf("dockerfilePath unset: %v", err)
	}
	if ok || path != "" || contextDir != "" {
		t.Fatalf("dockerfilePath unset = (%q, %q, %v), want zero values", path, contextDir, ok)
	}
}

func TestResolveStandaloneDockerfilePath(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	absPath := filepath.Join(t.TempDir(), "Dockerfile")

	tests := []struct {
		name        string
		path        string
		wantPath    string
		wantContext string
	}{
		{name: "unset"},
		{name: "relative", path: "build/Dockerfile", wantPath: filepath.Join(baseDir, "build", "Dockerfile"), wantContext: filepath.Join(baseDir, "build")},
		{name: "absolute", path: absPath, wantPath: absPath, wantContext: filepath.Dir(absPath)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotPath, gotContext := resolveStandaloneDockerfilePath(baseDir, tt.path)
			if gotPath != tt.wantPath || gotContext != tt.wantContext {
				t.Fatalf("resolveStandaloneDockerfilePath = (%q, %q), want (%q, %q)", gotPath, gotContext, tt.wantPath, tt.wantContext)
			}
		})
	}
}
