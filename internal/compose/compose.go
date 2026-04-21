package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/dockerfile"
	"github.com/zeroecco/holos/internal/images"
	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// File is the user-facing YAML compose format.
type File struct {
	Name     string             `yaml:"name"`
	Services map[string]Service `yaml:"services"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
}

// Service is a single VM definition within the compose file.
type Service struct {
	Image           string          `yaml:"image"`
	ImageFormat     string          `yaml:"image_format,omitempty"`
	Dockerfile      string          `yaml:"dockerfile,omitempty"`
	Replicas        int             `yaml:"replicas,omitempty"`
	VM              VM              `yaml:"vm,omitempty"`
	Ports           []string        `yaml:"ports,omitempty"`
	Volumes         []string        `yaml:"volumes,omitempty"`
	Devices         []ComposeDevice `yaml:"devices,omitempty"`
	DependsOn       []string        `yaml:"depends_on,omitempty"`
	CloudInit       CloudInit       `yaml:"cloud_init,omitempty"`
	StopGracePeriod string          `yaml:"stop_grace_period,omitempty"`
	Healthcheck     *Healthcheck    `yaml:"healthcheck,omitempty"`
}

// Healthcheck declares a liveness probe for a service. When set,
// `holos up` blocks on every dependent until this service reports
// healthy, mirroring docker-compose's `depends_on: condition:
// service_healthy` without requiring the verbose map form.
//
// The probe is a shell command run inside each replica over the
// project's auto-generated `holos exec` ssh key. Exit 0 is healthy;
// any other exit or a transport error counts as an attempt failure.
type Healthcheck struct {
	// Test is the shell command to run inside the VM. Accepts either
	// a YAML list (["pg_isready"]) or a single string ("curl -f
	// http://localhost").
	Test []string `yaml:"test,omitempty"`

	// Interval between probe attempts (e.g. "10s"). Defaults to 30s.
	Interval string `yaml:"interval,omitempty"`
	// Retries is how many consecutive failures count as unhealthy
	// AFTER start_period has elapsed. Defaults to 3.
	Retries int `yaml:"retries,omitempty"`
	// StartPeriod is a grace window right after boot during which
	// failures do not count toward `retries`. Defaults to 0 (no grace).
	StartPeriod string `yaml:"start_period,omitempty"`
	// Timeout bounds a single probe's wall-clock budget. Defaults
	// to 5s.
	Timeout string `yaml:"timeout,omitempty"`
}

// UnmarshalYAML accepts Healthcheck.Test as either a list of strings
// (canonical docker-compose form) or a single shell string. The single-
// string form is wrapped in ["sh", "-c", ...] so it runs through the
// shell exactly like docker-compose's CMD-SHELL variant.
func (h *Healthcheck) UnmarshalYAML(node *yaml.Node) error {
	type rawHealthcheck struct {
		Test        yaml.Node `yaml:"test"`
		Interval    string    `yaml:"interval"`
		Retries     int       `yaml:"retries"`
		StartPeriod string    `yaml:"start_period"`
		Timeout     string    `yaml:"timeout"`
	}
	var raw rawHealthcheck
	if err := node.Decode(&raw); err != nil {
		return err
	}

	h.Interval = raw.Interval
	h.Retries = raw.Retries
	h.StartPeriod = raw.StartPeriod
	h.Timeout = raw.Timeout

	switch raw.Test.Kind {
	case 0:
		// omitted
	case yaml.ScalarNode:
		var s string
		if err := raw.Test.Decode(&s); err != nil {
			return err
		}
		if s != "" {
			h.Test = []string{"sh", "-c", s}
		}
	case yaml.SequenceNode:
		var list []string
		if err := raw.Test.Decode(&list); err != nil {
			return err
		}
		h.Test = list
	default:
		return fmt.Errorf("healthcheck.test must be a string or list of strings")
	}
	return nil
}

// VM configures the virtual hardware for a service.
type VM struct {
	VCPU      int      `yaml:"vcpu,omitempty"`
	MemoryMB  int      `yaml:"memory_mb,omitempty"`
	Machine   string   `yaml:"machine,omitempty"`
	CPUModel  string   `yaml:"cpu_model,omitempty"`
	UEFI      bool     `yaml:"uefi,omitempty"`
	ExtraArgs []string `yaml:"extra_args,omitempty"`
}

// ComposeDevice is a PCI device to pass through to the VM via VFIO.
type ComposeDevice struct {
	PCI     string `yaml:"pci"`
	ROMFile string `yaml:"rom_file,omitempty"`
}

// CloudInit holds cloud-init configuration embedded in the compose file.
type CloudInit struct {
	User              string      `yaml:"user,omitempty"`
	SSHAuthorizedKeys []string    `yaml:"ssh_authorized_keys,omitempty"`
	Packages          []string    `yaml:"packages,omitempty"`
	WriteFiles        []WriteFile `yaml:"write_files,omitempty"`
	RunCmd            []string    `yaml:"runcmd,omitempty"`
	BootCmd           []string    `yaml:"bootcmd,omitempty"`
}

// WriteFile is a file to create inside the VM via cloud-init.
type WriteFile struct {
	Path        string `yaml:"path"`
	Content     string `yaml:"content"`
	Permissions string `yaml:"permissions,omitempty"`
	Owner       string `yaml:"owner,omitempty"`
}

// Volume configures a top-level named volume. Named volumes are
// qcow2-backed block devices that persist across `holos down`. They
// live under state_dir/volumes/<project>/<name>.qcow2 and are symlinked
// into each instance's workdir so teardown only removes the symlink.
type Volume struct {
	// Size is a human-friendly capacity like "10G", "500M", "1T".
	// Empty defaults to 10 GiB. The value is the VIRTUAL size of the
	// qcow2; on-disk usage grows sparsely with actual writes.
	Size string `yaml:"size,omitempty"`
}

// Project is the resolved, validated form ready for the runtime.
type Project struct {
	Name         string
	SpecHash     string
	ServiceOrder []string
	Services     map[string]config.Manifest
	Network      NetworkPlan
	// Volumes holds every named volume referenced anywhere in the
	// compose file, keyed by volume name. The runtime uses this to
	// pre-provision qcow2 backing files before any service starts.
	Volumes map[string]VolumeSpec
}

// VolumeSpec is the resolved form of a top-level named volume.
type VolumeSpec struct {
	Name      string
	SizeBytes int64
}

// NetworkPlan describes the internal network assigned to a project.
type NetworkPlan struct {
	MulticastGroup string
	MulticastPort  int
	Subnet         string
	Hosts          map[string]string
}

// DefaultFiles returns filenames to search for in priority order.
func DefaultFiles() []string {
	return []string{"holos.yaml", "holos.yml"}
}

// FindFile locates a compose file in the given directory.
func FindFile(dir string) (string, error) {
	for _, name := range DefaultFiles() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no holos.yaml found in %s", dir)
}

// Load reads and parses a compose file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}

	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	if file.Name == "" {
		abs, err := filepath.Abs(path)
		if err == nil {
			file.Name = filepath.Base(filepath.Dir(abs))
		}
	}

	return &file, nil
}

// Resolve validates the compose file and produces a Project.
// stateDir is used for the image cache when pulling remote images.
func (f *File) Resolve(baseDir string, stateDir string) (*Project, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}

	order, err := f.topoSort()
	if err != nil {
		return nil, err
	}

	network := f.planNetwork()

	hosts := make(map[string]string)
	ipCounter := 2
	serviceIPs := make(map[string][]string)

	for _, name := range order {
		svc := f.Services[name]
		replicas := svc.Replicas
		if replicas == 0 {
			replicas = 1
		}

		ips := make([]string, replicas)
		for i := 0; i < replicas; i++ {
			ip := fmt.Sprintf("10.10.0.%d", ipCounter)
			instanceName := fmt.Sprintf("%s-%d", name, i)
			hosts[instanceName] = ip
			ips[i] = ip
			ipCounter++
		}
		hosts[name] = ips[0]
		serviceIPs[name] = ips
	}
	network.Hosts = hosts

	cacheDir := images.DefaultCacheDir(stateDir)

	services := make(map[string]config.Manifest, len(f.Services))
	for _, name := range order {
		svc := f.Services[name]
		manifest, err := f.resolveService(name, svc, baseDir, cacheDir, network, hosts, serviceIPs[name])
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		services[name] = manifest
	}

	specHash, err := f.specHash()
	if err != nil {
		return nil, err
	}

	volumes, err := f.resolveVolumes(services)
	if err != nil {
		return nil, err
	}

	return &Project{
		Name:         f.Name,
		SpecHash:     specHash,
		ServiceOrder: order,
		Services:     services,
		Network:      network,
		Volumes:      volumes,
	}, nil
}

// resolveVolumes gathers every named volume actually referenced by a
// service and returns them with their resolved sizes. Unreferenced
// top-level volumes are intentionally omitted so `holos down` never
// leaves behind qcow2 files for volumes nothing asked for. A reference
// to a volume that's not declared is an error (prevents typos from
// silently degrading to bind mounts).
func (f *File) resolveVolumes(services map[string]config.Manifest) (map[string]VolumeSpec, error) {
	used := make(map[string]bool)
	for name, manifest := range services {
		for _, m := range manifest.Mounts {
			if m.Kind != config.MountKindVolume {
				continue
			}
			if _, ok := f.Volumes[m.VolumeName]; !ok {
				return nil, fmt.Errorf(
					"service %q references volume %q not declared in top-level volumes:",
					name, m.VolumeName)
			}
			used[m.VolumeName] = true
		}
	}

	if len(used) == 0 {
		return nil, nil
	}

	out := make(map[string]VolumeSpec, len(used))
	for name := range used {
		size, err := parseVolumeSize(f.Volumes[name].Size)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", name, err)
		}
		if !namePattern.MatchString(name) {
			return nil, fmt.Errorf("volume name %q must match %s", name, namePattern.String())
		}
		out[name] = VolumeSpec{Name: name, SizeBytes: size}
	}
	return out, nil
}

func (f *File) resolveService(name string, svc Service, baseDir string, cacheDir string, network NetworkPlan, hosts map[string]string, instanceIPs []string) (config.Manifest, error) {
	replicas := svc.Replicas
	if replicas == 0 {
		replicas = config.DefaultReplicas
	}

	ports, err := parsePorts(svc.Ports)
	if err != nil {
		return config.Manifest{}, err
	}

	mounts, err := parseVolumes(svc.Volumes, baseDir, f.Volumes)
	if err != nil {
		return config.Manifest{}, err
	}

	var dfWriteFiles []config.WriteFile
	var dfRunCmd []string
	if svc.Dockerfile != "" {
		dfPath := svc.Dockerfile
		if !filepath.IsAbs(dfPath) {
			dfPath = filepath.Join(baseDir, dfPath)
		}
		dfResult, err := dockerfile.Parse(dfPath, filepath.Dir(dfPath))
		if err != nil {
			return config.Manifest{}, fmt.Errorf("dockerfile: %w", err)
		}
		if svc.Image == "" && dfResult.FromImage != "" {
			svc.Image = dfResult.FromImage
		}
		dfWriteFiles = dfResult.WriteFiles
		dfRunCmd = []string{dockerfile.BuildCommand()}
	}

	image, imageFormat, err := resolveImage(svc.Image, svc.ImageFormat, baseDir, cacheDir)
	if err != nil {
		return config.Manifest{}, err
	}

	vcpu := svc.VM.VCPU
	if vcpu == 0 {
		vcpu = config.DefaultVCPU
	}
	memMB := svc.VM.MemoryMB
	if memMB == 0 {
		memMB = config.DefaultMemoryMB
	}
	machine := svc.VM.Machine
	if machine == "" {
		machine = config.DefaultMachine
	}
	cpuModel := svc.VM.CPUModel
	if cpuModel == "" {
		cpuModel = config.DefaultCPUModel
	}

	// User selection is a fallback chain:
	//   1. explicit cloud_init.user from the compose file
	//   2. the image's conventional cloud user (debian → "debian",
	//      alpine → "alpine", etc.) so cloud-init creates an account
	//      that matches what the rest of the ecosystem expects
	//   3. the global default ("ubuntu")
	// Using the image default in the middle slot is what keeps
	// `holos run debian` from producing a VM whose console autologin
	// fails because no `ubuntu` user materialised.
	user := svc.CloudInit.User
	if user == "" {
		user = images.DefaultUser(svc.Image)
	}
	if user == "" {
		user = config.DefaultUser
	}

	writeFiles := make([]config.WriteFile, 0, len(dfWriteFiles)+len(svc.CloudInit.WriteFiles))
	writeFiles = append(writeFiles, dfWriteFiles...)
	for _, wf := range svc.CloudInit.WriteFiles {
		perms := wf.Permissions
		if perms == "" {
			perms = "0644"
		}
		owner := wf.Owner
		if owner == "" {
			owner = "root:root"
		}
		writeFiles = append(writeFiles, config.WriteFile{
			Path:        wf.Path,
			Content:     wf.Content,
			Permissions: perms,
			Owner:       owner,
		})
	}

	baseMAC := generateMAC(0x00, f.Name, name)

	devices := make([]config.Device, len(svc.Devices))
	for i, d := range svc.Devices {
		devices[i] = config.Device{
			PCI:     normalizePCIAddress(d.PCI),
			ROMFile: d.ROMFile,
		}
	}

	uefi := svc.VM.UEFI
	if !uefi && len(devices) > 0 {
		uefi = true
	}

	gracePeriodSec, err := parseStopGracePeriod(svc.StopGracePeriod)
	if err != nil {
		return config.Manifest{}, err
	}

	healthcheck, err := resolveHealthcheck(svc.Healthcheck)
	if err != nil {
		return config.Manifest{}, err
	}

	return config.Manifest{
		APIVersion:  "holos/v1alpha1",
		Kind:        "Service",
		Name:        name,
		Replicas:    replicas,
		Image:       image,
		ImageFormat: imageFormat,
		VM: config.VMConfig{
			VCPU:      vcpu,
			MemoryMB:  memMB,
			Machine:   machine,
			CPUModel:  cpuModel,
			UEFI:      uefi,
			ExtraArgs: svc.VM.ExtraArgs,
		},
		Devices: devices,
		Network: config.NetworkConfig{Mode: "user"},
		Ports:   ports,
		Mounts:  mounts,
		CloudInit: config.CloudInit{
			User:              user,
			SSHAuthorizedKeys: svc.CloudInit.SSHAuthorizedKeys,
			Packages:          svc.CloudInit.Packages,
			WriteFiles:        writeFiles,
			RunCmd:            append(dfRunCmd, svc.CloudInit.RunCmd...),
			BootCmd:           svc.CloudInit.BootCmd,
		},
		InternalNetwork: &config.InternalNetworkConfig{
			MulticastGroup: network.MulticastGroup,
			MulticastPort:  network.MulticastPort,
			Subnet:         network.Subnet,
			InstanceIPs:    instanceIPs,
			BaseMAC:        baseMAC,
			UserBaseMAC:    generateMAC(0x01, f.Name, name),
		},
		ExtraHosts:         hosts,
		StopGracePeriodSec: gracePeriodSec,
		Healthcheck:        healthcheck,
		DependsOn:          append([]string(nil), svc.DependsOn...),
	}, nil
}

// resolveHealthcheck validates and normalises a compose healthcheck
// block into the resolved config form. Absent blocks pass through as
// nil so consumers never have to check zero-value fields.
func resolveHealthcheck(h *Healthcheck) (*config.HealthcheckConfig, error) {
	if h == nil {
		return nil, nil
	}
	if len(h.Test) == 0 {
		return nil, fmt.Errorf("healthcheck.test is required")
	}
	intervalSec, err := parseDurationSec(h.Interval, config.DefaultHealthIntervalSec)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.interval: %w", err)
	}
	startSec, err := parseDurationSec(h.StartPeriod, 0)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.start_period: %w", err)
	}
	timeoutSec, err := parseDurationSec(h.Timeout, config.DefaultHealthTimeoutSec)
	if err != nil {
		return nil, fmt.Errorf("healthcheck.timeout: %w", err)
	}
	retries := h.Retries
	if retries == 0 {
		retries = config.DefaultHealthRetries
	}
	return &config.HealthcheckConfig{
		Test:           append([]string{}, h.Test...),
		IntervalSec:    intervalSec,
		Retries:        retries,
		StartPeriodSec: startSec,
		TimeoutSec:     timeoutSec,
	}, nil
}

// parseDurationSec accepts a Go duration string and returns whole
// seconds, returning the fallback when the input is empty. Values
// below 1s round up to 1s so healthcheck loops never busy-spin on
// fractional intervals.
func parseDurationSec(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %q must be non-negative", raw)
	}
	seconds := int(d.Seconds())
	if d > 0 && seconds < 1 {
		seconds = 1
	}
	return seconds, nil
}

// parseStopGracePeriod accepts a Go duration string (e.g. "30s", "2m") and
// returns it as whole seconds. Empty string yields 0 so callers can apply
// their own default.
func parseStopGracePeriod(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("stop_grace_period %q: %w", raw, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("stop_grace_period %q: must be non-negative", raw)
	}
	seconds := int(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return seconds, nil
}

func (f *File) validate() error {
	if !namePattern.MatchString(f.Name) {
		return fmt.Errorf("project name %q must match %s", f.Name, namePattern.String())
	}
	if len(f.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}
	for name, svc := range f.Services {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("service name %q must match %s", name, namePattern.String())
		}
		if svc.Image == "" && svc.Dockerfile == "" {
			return fmt.Errorf("service %q requires an image (or a dockerfile with FROM)", name)
		}
		for _, dep := range svc.DependsOn {
			if _, ok := f.Services[dep]; !ok {
				return fmt.Errorf("service %q depends on unknown service %q", name, dep)
			}
		}
	}
	return nil
}

func (f *File) topoSort() ([]string, error) {
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)

	for name := range f.Services {
		inDegree[name] = 0
	}
	for name, svc := range f.Services {
		for _, dep := range svc.DependsOn {
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		deps := dependents[node]
		sort.Strings(deps)
		for _, dep := range deps {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(f.Services) {
		return nil, fmt.Errorf("circular dependency detected among services")
	}
	return order, nil
}

// planNetwork derives the multicast group and port for a project's internal
// network from a SHA-256 of the project name. Using a cryptographic hash
// across three group octets and the port gives ~40 bits of entropy, which
// makes accidental collisions between unrelated stacks on the same host
// vanishingly unlikely.
//
// The group is drawn from the IPv4 administratively-scoped range
// 239.0.0.0/8 (RFC 2365), which is intended for local use and is not
// forwarded outside the host.
func (f *File) planNetwork() NetworkPlan {
	sum := sha256.Sum256([]byte(f.Name))

	group := fmt.Sprintf("239.%d.%d.%d", sum[0], sum[1], sum[2])
	portBase := uint16(sum[3])<<8 | uint16(sum[4])
	port := 10000 + int(portBase)%55000

	return NetworkPlan{
		MulticastGroup: group,
		MulticastPort:  port,
		Subnet:         "10.10.0.0/24",
	}
}

func (f *File) specHash() (string, error) {
	data, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// generateMAC produces a locally-administered unicast MAC derived from the
// SHA-256 of the project and service names. The layout is:
//
//	52:54:<prefix>:<h0>:<h1>:00
//
// where prefix distinguishes the internal NIC (0x00) from the user NIC
// (0x01), h0/h1 are two bytes of SHA-256 entropy, and the last octet is
// reserved for the per-replica offset applied by InstanceMAC.
//
// Cross-project MAC collision risk is bounded by the multicast
// group+port pair (~40 bits of entropy): two VMs only share an L2 segment
// when their projects collide in BOTH group and port.
func generateMAC(prefix byte, project, service string) string {
	sum := sha256.Sum256([]byte(project + "/" + service))
	return fmt.Sprintf("52:54:%02x:%02x:%02x:00", prefix, sum[0], sum[1])
}

func parsePorts(specs []string) ([]config.PortForward, error) {
	ports := make([]config.PortForward, 0, len(specs))
	for i, spec := range specs {
		port, err := parsePort(spec)
		if err != nil {
			return nil, fmt.Errorf("port %q: %w", spec, err)
		}
		if port.Name == "" {
			port.Name = fmt.Sprintf("port-%d", i)
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func parsePort(spec string) (config.PortForward, error) {
	protocol := "tcp"
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}
	// Only TCP forwarding is implemented end-to-end; reject other
	// protocols at parse time rather than let the user discover the
	// limitation at `holos up` via a validation error.
	if protocol != "tcp" {
		return config.PortForward{}, fmt.Errorf("protocol %q is unsupported; only tcp is implemented", protocol)
	}

	parts := strings.SplitN(spec, ":", 2)
	switch len(parts) {
	case 1:
		guest, err := strconv.Atoi(parts[0])
		if err != nil {
			return config.PortForward{}, fmt.Errorf("invalid port: %w", err)
		}
		return config.PortForward{GuestPort: guest, Protocol: protocol}, nil
	case 2:
		host, err := strconv.Atoi(parts[0])
		if err != nil {
			return config.PortForward{}, fmt.Errorf("invalid host port: %w", err)
		}
		guest, err := strconv.Atoi(parts[1])
		if err != nil {
			return config.PortForward{}, fmt.Errorf("invalid guest port: %w", err)
		}
		return config.PortForward{HostPort: host, GuestPort: guest, Protocol: protocol}, nil
	default:
		return config.PortForward{}, fmt.Errorf("invalid port spec")
	}
}

func parseVolumes(specs []string, baseDir string, declared map[string]Volume) ([]config.Mount, error) {
	mounts := make([]config.Mount, 0, len(specs))
	for _, spec := range specs {
		mount, err := parseVolume(spec, baseDir, declared)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", spec, err)
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

// parseVolume splits a compose-style volume spec ("source:target[:ro]")
// into a typed Mount. Sources that match a declared top-level volume are
// treated as named (block) volumes; everything else is a host bind mount
// (virtfs), preserving existing behavior.
func parseVolume(spec string, baseDir string, declared map[string]Volume) (config.Mount, error) {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) < 2 {
		return config.Mount{}, fmt.Errorf("volume requires source:target")
	}

	source := parts[0]
	target := parts[1]
	readOnly := len(parts) == 3 && parts[2] == "ro"

	if vol, ok := declared[source]; ok {
		sizeBytes, err := parseVolumeSize(vol.Size)
		if err != nil {
			return config.Mount{}, fmt.Errorf("volume %q: %w", source, err)
		}
		return config.Mount{
			Kind:       config.MountKindVolume,
			VolumeName: source,
			SizeBytes:  sizeBytes,
			Target:     target,
			ReadOnly:   readOnly,
		}, nil
	}

	// Distinguish bind mounts from named volumes the same way docker
	// compose does: anything that looks like a path (absolute, ./,
	// ../, or containing a separator) is a bind mount; anything else
	// is a named-volume reference that must match a declared volume.
	// Treating a bare identifier as an implicit relative bind mount
	// would mask typos like `dta:/mnt`, so we reject it explicitly.
	if !looksLikePath(source) {
		return config.Mount{}, fmt.Errorf(
			"volume source %q is not a declared top-level volume and does not look like a path; "+
				"add it under volumes: or prefix with ./ for a bind mount",
			source)
	}

	if !filepath.IsAbs(source) {
		source = filepath.Join(baseDir, source)
		if abs, err := filepath.Abs(source); err == nil {
			source = abs
		}
	}

	return config.Mount{
		Kind:     config.MountKindBind,
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}, nil
}

// looksLikePath returns true for strings a user would expect to be
// interpreted as filesystem paths: absolute paths, explicit ./ or ../
// roots, or anything containing a path separator. Bare identifiers
// ("data", "cache") are treated as named-volume references.
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	if filepath.IsAbs(s) {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	return strings.ContainsRune(s, os.PathSeparator)
}

// parseVolumeSize accepts a human-friendly size string (case-insensitive):
// plain bytes ("1048576"), or a decimal with a unit suffix: K/M/G/T (binary
// multipliers, matching qemu-img convention). Empty returns the default.
func parseVolumeSize(raw string) (int64, error) {
	if raw == "" {
		return defaultVolumeSizeBytes, nil
	}

	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return defaultVolumeSizeBytes, nil
	}

	multiplier := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'K':
		multiplier = 1 << 10
	case 'M':
		multiplier = 1 << 20
	case 'G':
		multiplier = 1 << 30
	case 'T':
		multiplier = 1 << 40
	}
	if multiplier != 1 {
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q (expected e.g. \"10G\"): %w", raw, err)
	}
	bytes := int64(value * float64(multiplier))
	if bytes < minVolumeSizeBytes {
		return 0, fmt.Errorf("volume size %q is below minimum %d bytes", raw, minVolumeSizeBytes)
	}
	return bytes, nil
}

const (
	// defaultVolumeSizeBytes is the virtual size used when a named
	// volume omits an explicit `size:` field. Matches docker's "what
	// you'd get if you didn't think about it" convention.
	defaultVolumeSizeBytes = 10 * (1 << 30) // 10 GiB

	// minVolumeSizeBytes is a sanity floor; below this qemu-img
	// rounding produces surprising results and most filesystems can't
	// even hold their own superblock.
	minVolumeSizeBytes = 1 * (1 << 20) // 1 MiB
)

func resolveImage(ref string, explicitFormat string, baseDir string, cacheDir string) (path string, format string, err error) {
	path, format, err = images.Pull(ref, cacheDir)
	if err != nil {
		return "", "", err
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}

	if explicitFormat != "" {
		format = explicitFormat
	}
	return path, format, nil
}

func normalizePCIAddress(addr string) string {
	if strings.Count(addr, ":") == 1 {
		return "0000:" + addr
	}
	return addr
}
