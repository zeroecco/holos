package compose

import (
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
