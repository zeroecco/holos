package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveAcceptsComposeBooleanCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: booleans
services:
  vm:
    image: ./base.qcow2
    init: "true"
    privileged: "true"
    read_only: "false"
    tty: "true"
    stdin_open: "true"
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := len(project.Services["vm"].Devices); got != 0 {
		t.Fatalf("compose string devices should be compatibility metadata, got %d passthrough devices", got)
	}
}

func TestResolveAcceptsComposeLifecycleCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: lifecycle
services:
  vm:
    image: ./base.qcow2
    container_name: lifecycle-vm
    platform: linux/amd64
    pull_policy: missing
    profiles: ["debug", "local"]
    restart: unless-stopped
    stop_signal: SIGTERM
    oom_kill_disable: "true"
    pids_limit: "128"
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := len(project.Services["vm"].Devices); got != 0 {
		t.Fatalf("compose string devices should be compatibility metadata, got %d passthrough devices", got)
	}
}

func TestResolveAcceptsComposeNetworkCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: networkcompat
services:
  api:
    image: ./base.qcow2
    annotations:
      com.example.owner: platform
    dns: 1.1.1.1
    dns_search:
      - svc.local
      - example.internal
    expose:
      - "8080"
    external_links:
      - redis
    group_add:
      - dialout
      - "1000"
    links:
      - db
  worker:
    image: ./base.qcow2
    annotations:
      - com.example.role=worker
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestResolveAcceptsComposeNetworksSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: networks
services:
  api:
    image: ./base.qcow2
    networks:
      frontend:
        aliases: [web]
        ipv4_address: 172.20.0.10
        mac_address: 02:42:ac:14:00:0a
        priority: 10.5
        gw_priority: 1.5
        backend: {}
  worker:
    image: ./base.qcow2
    networks:
      - backend
networks:
  frontend:
    driver: bridge
    driver_opts:
      com.docker.network.bridge.name: holos0
      mtu: 1500
    labels:
      com.example.tier: edge
    ipam:
      config:
        - subnet: 172.20.0.0/16
          gateway: 172.20.0.1
  backend:
    internal: true
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestDecodeServiceNetworkListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		node    yaml.Node
		want    string
		wantErr string
	}{
		{name: "scalar", node: yaml.Node{Kind: yaml.ScalarNode, Value: "backend"}, want: "backend"},
		{name: "mapping", node: yaml.Node{Kind: yaml.MappingNode, Line: 12}, wantErr: "line 12: network list entries must be scalar values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeServiceNetworkListItem(&tt.node)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeServiceNetworkListItem: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeServiceNetworkListItem = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeServiceNetworkMapValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantAlias   string
		wantDefault bool
		wantErr     string
	}{
		{name: "null", raw: "null", wantDefault: true},
		{name: "mapping", raw: "aliases: [web]", wantAlias: "web"},
		{name: "invalid", raw: "backend", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal YAML: %v", err)
			}
			got, err := decodeServiceNetworkMapValue(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeServiceNetworkMapValue: %v", err)
			}
			if tt.wantDefault {
				assertStringSliceEqual(t, "aliases", got.Aliases, nil)
				if got.IPv4Address != "" {
					t.Fatalf("decodeServiceNetworkMapValue = %+v, want zero value", got)
				}
				return
			}
			assertStringSliceEqual(t, "aliases", got.Aliases, []string{tt.wantAlias})
		})
	}
}

func TestResolveAcceptsComposeConfigsAndSecretsSyntax(t *testing.T) {
	dir := t.TempDir()
	writeTestImage(t, dir)
	writeTestFile(t, dir, "app.conf", "fake")
	writeTestFile(t, dir, "token.txt", "fake")
	t.Setenv("API_TOKEN", "fake")
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
      - db_password
      - source: api_token
        target: /run/secrets/api_token
configs:
  app_config:
    file: ./app.conf
    labels:
      com.example.kind: config
  generated:
    content: hello
    template_driver: golang
secrets:
  db_password:
    file: ./token.txt
    labels:
      - com.example.kind=secret
    driver: example-driver
    driver_opts:
      region: test
    template_driver: golang
  api_token:
    environment: API_TOKEN
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestResolveAcceptsComposeRuntimeCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: runtimecompat
services:
  vm:
    image: ./base.qcow2
    cap_add: [NET_ADMIN]
    cap_drop:
      - MKNOD
    cgroup: private
    cgroup_parent: m-executor-abcd
    cpu_count: 2
    cpu_percent: 50
    cpu_period: "100000"
    cpu_quota: "50000"
    cpu_rt_period: 400ms
    cpu_rt_runtime: 95000
    cpu_shares: "512"
    cpuset: "0-1"
    credential_spec:
      file: my-credential-spec.json
    isolation: default
    ipc: host
    mem_reservation: 512M
    mem_swappiness: "10"
    memswap_limit: 2G
    oom_score_adj: "100"
    pid: host
    runtime: runc
    security_opt:
      - label:disable
    shm_size: 64M
    storage_opt:
      size: 1G
    sysctls:
      net.core.somaxconn: "1024"
    tmpfs:
      - /run
    ulimits:
      nofile:
        soft: 20000
        hard: 40000
      nproc: 65535
    uts: host
    userns_mode: host
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestResolveAcceptsComposeMiscCompatibilitySyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
version: "3.9"
name: misccompat
include:
  - common.yaml
services:
  vm:
    image: ./base.qcow2
    attach: "false"
    pull_policy: refresh
    pull_refresh_after: 1h30m
    blkio_config:
      weight: "300"
      weight_device:
        - path: /dev/sda
          weight: "400"
      device_read_bps:
        - path: /dev/sda
          rate: 12mb
      device_write_iops:
        - path: /dev/sda
          rate: 120
    device_cgroup_rules:
      - 'c 1:3 mr'
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
      - vendor1.com/device=gpu
      - source: /dev/fuse
        target: /dev/fuse
        permissions: rwm
    device_read_bps:
      - path: /dev/sdb
        rate: 1mb
    device_read_iops:
      - path: /dev/sdb
        rate: 100
    device_write_bps:
      - path: /dev/sdb
        rate: 2mb
    device_write_iops:
      - path: /dev/sdb
        rate: 200
    dns_opt:
      - use-vc
    develop:
      watch:
        - path: ./src
          action: sync
          include:
            - "*.go"
          target: /app
          exec:
            command: echo synced
            privileged: "true"
    extends:
      file: common.yaml
      service: base
    gpus:
      - driver: 3dfx
        count: 2
        capabilities: [gpu]
        device_ids:
          - GPU-123
        options:
          - profile=compute
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: 3
    mac_address: 02:42:ac:11:00:02
    post_start:
      - command: echo started
        user: root
        privileged: "true"
        environment:
          HOOK: post
    pre_stop:
      - command: echo stopping
    provider:
      type: awesomecloud
      options:
        size: small
        regions:
          - us-west
          - us-east
    use_api_socket: true
    volumes_from:
      - db:ro
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestResolveAcceptsComposeModelsSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: models
services:
  app:
    image: ./base.qcow2
    models:
      llm:
        endpoint_var: MODEL_URL
        model_var: MODEL_NAME
  worker:
    image: ./base.qcow2
    models:
      - embeddings
models:
  llm:
    name: logical-llm
    model: ai/example
    context_size: 4096
    runtime_flags:
      - --threads=4
  embeddings:
    model: ai/embed
`
	resolveTestCompose(t, dir, yamlDoc)
}

func TestLoadAcceptsComposeExtensionFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
x-project: common metadata
name: extensions
services:
  vm:
    x-service:
      team: platform
    image: ./base.qcow2
    build:
      context: .
      dockerfile_inline: |
        RUN echo extension
      x-bake:
        platforms:
          - linux/amd64
    deploy:
      x-swarm-note: ignored
      resources:
        limits:
          cpus: "1.5"
networks:
  default:
    x-network: ignored
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := project.Services["vm"].VM.VCPU; got != 2 {
		t.Fatalf("vcpu = %d, want deploy cpu fallback after stripping extensions", got)
	}
}

func TestLoadAcceptsComposeResetAndOverrideTags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: tags
services:
  vm:
    image: ./base.qcow2
    ports: !override
      - "8080:80"
    environment: !reset null
`
	project := resolveTestCompose(t, dir, yamlDoc)
	if got := len(project.Services["vm"].Ports); got != 1 {
		t.Fatalf("ports len = %d, want 1", got)
	}
}

func TestLoadAcceptsComposeAnchorsAndAliases(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: anchors
x-common-env: &common-env
  APP_ENV: test
services:
  vm:
    image: ./base.qcow2
    environment: *common-env
`
	project := resolveTestCompose(t, dir, yamlDoc)
	assertEnvironmentFile(t, "vm", project, []string{testEnvAppKey + `="test"`})
}
