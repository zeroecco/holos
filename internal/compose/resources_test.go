package compose

import (
	"os"
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

func assertServiceResource(t *testing.T, got ServiceResource, source, target string) {
	t.Helper()

	if got.Source != source || got.Target != target {
		t.Fatalf("service resource = %+v, want source %q target %q", got, source, target)
	}
}

func TestResolveAcceptsComposeResourceSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: resources
services:
  vm:
    image: ./base.qcow2
    cpus: "2.5"
    mem_limit: 1G
`
	project := resolveTestCompose(t, dir, yamlDoc)
	vm := project.Services["vm"].VM
	if vm.VCPU != 3 {
		t.Fatalf("vcpu = %d, want ceil(2.5) = 3", vm.VCPU)
	}
	if vm.MemoryMB != 1024 {
		t.Fatalf("memory_mb = %d, want 1024", vm.MemoryMB)
	}
}

func TestResolveAcceptsComposeDeploySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: deploy
services:
  worker:
    image: ./base.qcow2
    deploy:
      mode: replicated
      replicas: 3
      endpoint_mode: vip
      labels:
        com.example.role: worker
      resources:
        limits:
          cpus: "2.5"
          memory: 1G
          pids: "64"
        reservations:
          cpus: "1.0"
          memory: 512M
          generic_resources:
            - discrete_resource_spec:
                kind: FPGA
                value: "2"
          devices:
            - capabilities: [gpu]
              driver: nvidia
              count: all
              device_ids:
                - GPU-123
              options:
                - virtualization=false
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
        window: 30s
      placement:
        constraints:
          - node.labels.zone == west
      update_config:
        parallelism: 1
        delay: 10s
      rollback_config:
        parallelism: 1
`
	project := resolveTestCompose(t, dir, yamlDoc)
	service := project.Services["worker"]
	if service.Replicas != 3 {
		t.Fatalf("replicas = %d, want 3 from deploy.replicas", service.Replicas)
	}
	if service.VM.VCPU != 3 {
		t.Fatalf("vcpu = %d, want ceil(deploy.resources.limits.cpus)", service.VM.VCPU)
	}
	if service.VM.MemoryMB != 1024 {
		t.Fatalf("memory_mb = %d, want deploy.resources.limits.memory", service.VM.MemoryMB)
	}
}

func TestDecodeServiceResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantSource string
		wantTarget string
		wantErr    string
	}{
		{name: "scalar", raw: "app_config", wantSource: "app_config"},
		{name: "mapping", raw: "source: app_config\ntarget: /etc/app.conf\n", wantSource: "app_config", wantTarget: "/etc/app.conf"},
		{name: "invalid", raw: "[app_config]", wantErr: "resource references must be strings or mappings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal YAML: %v", err)
			}
			got, err := decodeServiceResource(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeServiceResource: %v", err)
			}
			assertServiceResource(t, got, tt.wantSource, tt.wantTarget)
		})
	}
}

func TestResolveResourceWriteFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "app.conf", "from file\n")
	writeTestFile(t, dir, "password.txt", "secret file\n")
	t.Setenv("INLINE_SECRET", "from env\n")

	svc := Service{
		Configs: ServiceResources{
			{Source: "file_config", Target: "/etc/app.conf", UID: "1000", GID: "1001", Mode: 0440},
			{Source: "inline_config"},
		},
		Secrets: ServiceResources{
			{Source: "password"},
			{Source: "token", Target: "api_token", UID: "app", Mode: "0440"},
		},
	}
	configs := map[string]Config{
		"file_config":   {File: "./app.conf"},
		"inline_config": {Content: "inline\n"},
	}
	secrets := map[string]Secret{
		"password": {File: "./password.txt"},
		"token":    {Environment: "INLINE_SECRET"},
	}

	got, err := resolveResourceWriteFiles(dir, svc, configs, secrets)
	if err != nil {
		t.Fatalf("resolveResourceWriteFiles: %v", err)
	}
	want := []config.WriteFile{
		{Path: "/etc/app.conf", Content: "from file\n", Permissions: "0440", Owner: "1000:1001"},
		{Path: "/inline_config", Content: "inline\n", Permissions: configDefaultPermissions, Owner: config.DefaultFileOwner},
		{Path: "/run/secrets/password", Content: "secret file\n", Permissions: secretDefaultPermissions, Owner: config.DefaultFileOwner},
		{Path: "/run/secrets/api_token", Content: "from env\n", Permissions: "0440", Owner: "app:root"},
	}
	if len(got) != len(want) {
		t.Fatalf("resource write_files len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resource write_files[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveResourceWriteFilesErrors(t *testing.T) {
	tests := []struct {
		name    string
		svc     Service
		configs map[string]Config
		secrets map[string]Secret
		wantErr string
	}{
		{
			name:    "missing config declaration",
			svc:     Service{Configs: ServiceResources{{Source: "missing"}}},
			wantErr: `config "missing" is not declared`,
		},
		{
			name:    "external config",
			svc:     Service{Configs: ServiceResources{{Source: "external"}}},
			configs: map[string]Config{"external": {External: true}},
			wantErr: `config "external" is external`,
		},
		{
			name:    "missing secret environment",
			svc:     Service{Secrets: ServiceResources{{Source: "token"}}},
			secrets: map[string]Secret{"token": {Environment: "MISSING_SECRET_ENV"}},
			wantErr: `environment variable "MISSING_SECRET_ENV" is not set`,
		},
		{
			name:    "invalid mode",
			svc:     Service{Secrets: ServiceResources{{Source: "token", Mode: "bad"}}},
			secrets: map[string]Secret{"token": {Environment: "TOKEN_ENV"}},
			wantErr: `mode "bad" must be octal`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOKEN_ENV", "secret")

			_, err := resolveResourceWriteFiles(t.TempDir(), tt.svc, tt.configs, tt.secrets)
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestResolveComposeConfigsAndSecretsIntoWriteFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, "app.conf", "file config\n")
	t.Setenv("DB_PASSWORD", "env secret\n")
	yamlDoc := `
name: configsecrets
services:
  api:
    image: ./base.qcow2
    configs:
      - source: app_config
        target: /etc/app.conf
        uid: "1000"
        gid: "1000"
        mode: 0440
    secrets:
      - source: db_password
        target: /run/secrets/db_password
configs:
  app_config:
    file: ./app.conf
secrets:
  db_password:
    environment: DB_PASSWORD
`
	project := resolveTestCompose(t, dir, yamlDoc)
	writeFiles := project.Services[testComposeAPIService].CloudInit.WriteFiles
	assertWriteFileContains(t, testComposeAPIService, "/etc/app.conf", writeFiles, "file config")
	assertWriteFileContains(t, testComposeAPIService, "/run/secrets/db_password", writeFiles, "env secret")
	assertWriteFileMetadata(t, "/etc/app.conf", writeFiles, "0440", "1000:1000")
	assertWriteFileMetadata(t, "/run/secrets/db_password", writeFiles, secretDefaultPermissions, config.DefaultFileOwner)
}

func assertWriteFileMetadata(t *testing.T, path string, writeFiles []config.WriteFile, permissions, owner string) {
	t.Helper()

	for _, file := range writeFiles {
		if file.Path != path {
			continue
		}
		if file.Permissions != permissions || file.Owner != owner {
			t.Fatalf("%s metadata = permissions %q owner %q, want %q %q", path, file.Permissions, file.Owner, permissions, owner)
		}
		return
	}
	t.Fatalf("missing write file %s", path)
}

func TestResourcePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     any
		fallback string
		want     string
		wantErr  string
	}{
		{name: "nil", fallback: "0444", want: "0444"},
		{name: "yaml octal int", mode: 0440, want: "0440"},
		{name: "string", mode: "440", want: "0440"},
		{name: "empty string", mode: " ", fallback: "0400", want: "0400"},
		{name: "bad string", mode: "bad", wantErr: "must be octal"},
		{name: "float", mode: 1.5, wantErr: "must be an integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resourcePermissions(tt.mode, tt.fallback)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("resourcePermissions: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resourcePermissions = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceExternal(t *testing.T) {
	t.Parallel()

	if resourceExternal(nil) {
		t.Fatal("nil external = true, want false")
	}
	if resourceExternal(false) {
		t.Fatal("false external = true, want false")
	}
	if !resourceExternal(true) {
		t.Fatal("true external = false, want true")
	}
	if !resourceExternal(map[string]string{"name": "prod"}) {
		t.Fatal("map external = false, want true")
	}
}

func TestReadResourceFileUsesAbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeTestFile(t, dir, "resource.txt", "body")
	got, err := readResourceFile("/wrong/base", path)
	if err != nil {
		t.Fatalf("readResourceFile absolute: %v", err)
	}
	if got != "body" {
		t.Fatalf("readResourceFile = %q, want body", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture path disappeared: %v", err)
	}
}

func TestComposeCPUs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		svc     Service
		want    float64
		wantErr string
	}{
		{name: "service cpus", svc: Service{CPUs: "2.5"}, want: 2.5},
		{name: "deploy limits", svc: Service{Deploy: Deploy{Resources: DeployResources{
			Limits:       DeployResource{CPUs: "2"},
			Reservations: DeployResource{CPUs: "1.5"},
		}}}, want: 2},
		{name: "deploy reservations fallback", svc: Service{Deploy: Deploy{Resources: DeployResources{
			Limits:       DeployResource{CPUs: " "},
			Reservations: DeployResource{CPUs: "1.5"},
		}}}, want: 1.5},
		{name: "invalid deploy limits", svc: Service{Deploy: Deploy{Resources: DeployResources{
			Limits: DeployResource{CPUs: "fast"},
		}}}, wantErr: "deploy.resources.limits.cpus:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := composeCPUs(tt.svc)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("composeCPUs: %v", err)
			}
			if got != tt.want {
				t.Fatalf("composeCPUs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComposeVCPU(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cpus float64
		want int
	}{
		{cpus: 0, want: config.DefaultVCPU},
		{cpus: -1, want: config.DefaultVCPU},
		{cpus: 1.0, want: 1},
		{cpus: 1.1, want: 2},
	}
	for _, tt := range tests {
		if got := composeVCPU(tt.cpus); got != tt.want {
			t.Fatalf("composeVCPU(%v) = %d, want %d", tt.cpus, got, tt.want)
		}
	}
}

func TestComposeMemLimitPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		svc  Service
		want string
	}{
		{name: "service mem_limit", svc: Service{
			MemLimit: "2G",
			Deploy: Deploy{Resources: DeployResources{
				Limits:       DeployResource{Memory: "1G"},
				Reservations: DeployResource{Memory: "512M"},
			}},
		}, want: "2G"},
		{name: "deploy limits", svc: Service{
			MemLimit: "  ",
			Deploy: Deploy{Resources: DeployResources{
				Limits:       DeployResource{Memory: "1G"},
				Reservations: DeployResource{Memory: "512M"},
			}},
		}, want: "1G"},
		{name: "deploy reservations fallback", svc: Service{
			Deploy: Deploy{Resources: DeployResources{
				Limits:       DeployResource{Memory: "\t"},
				Reservations: DeployResource{Memory: "512M"},
			}},
		}, want: "512M"},
		{name: "all blank", svc: Service{
			MemLimit: " ",
			Deploy: Deploy{Resources: DeployResources{
				Limits:       DeployResource{Memory: "\t"},
				Reservations: DeployResource{Memory: ""},
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := composeMemLimit(tt.svc); got != tt.want {
				t.Fatalf("composeMemLimit = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComposeMemoryMB(t *testing.T) {
	t.Parallel()

	got, err := composeMemoryMB("")
	if err != nil {
		t.Fatalf("composeMemoryMB default: %v", err)
	}
	if got != config.DefaultMemoryMB {
		t.Fatalf("composeMemoryMB default = %d, want %d", got, config.DefaultMemoryMB)
	}

	got, err = composeMemoryMB("1537K")
	if err != nil {
		t.Fatalf("composeMemoryMB rounded: %v", err)
	}
	if got != 2 {
		t.Fatalf("composeMemoryMB rounded = %d, want 2", got)
	}
}

func TestBytesToMiBRoundedUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bytes int64
		want  int
	}{
		{name: "zero", bytes: 0, want: 0},
		{name: "exact", bytes: 2 * (1 << 20), want: 2},
		{name: "rounded up", bytes: (2 * (1 << 20)) + 1, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := bytesToMiBRoundedUp(tt.bytes); got != tt.want {
				t.Fatalf("bytesToMiBRoundedUp(%d) = %d, want %d", tt.bytes, got, tt.want)
			}
		})
	}
}
