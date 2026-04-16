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

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/dockerfile"
	"github.com/zeroecco/holos/internal/images"
	"gopkg.in/yaml.v3"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// File is the user-facing YAML compose format.
type File struct {
	Name     string              `yaml:"name"`
	Services map[string]Service  `yaml:"services"`
	Volumes  map[string]Volume   `yaml:"volumes,omitempty"`
}

// Service is a single VM definition within the compose file.
type Service struct {
	Image       string          `yaml:"image"`
	ImageFormat string          `yaml:"image_format,omitempty"`
	Dockerfile  string          `yaml:"dockerfile,omitempty"`
	Replicas    int             `yaml:"replicas,omitempty"`
	VM          VM              `yaml:"vm,omitempty"`
	Ports       []string        `yaml:"ports,omitempty"`
	Volumes     []string        `yaml:"volumes,omitempty"`
	Devices     []ComposeDevice `yaml:"devices,omitempty"`
	DependsOn   []string        `yaml:"depends_on,omitempty"`
	CloudInit   CloudInit       `yaml:"cloud_init,omitempty"`
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

// Volume is a placeholder for top-level named volumes (reserved for future use).
type Volume struct{}

// Project is the resolved, validated form ready for the runtime.
type Project struct {
	Name         string
	SpecHash     string
	ServiceOrder []string
	Services     map[string]config.Manifest
	Network      NetworkPlan
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

	return &Project{
		Name:         f.Name,
		SpecHash:     specHash,
		ServiceOrder: order,
		Services:     services,
		Network:      network,
	}, nil
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

	mounts, err := parseVolumes(svc.Volumes, baseDir)
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

	user := svc.CloudInit.User
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
		ExtraHosts: hosts,
	}, nil
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

func parseVolumes(specs []string, baseDir string) ([]config.Mount, error) {
	mounts := make([]config.Mount, 0, len(specs))
	for _, spec := range specs {
		mount, err := parseVolume(spec, baseDir)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", spec, err)
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func parseVolume(spec string, baseDir string) (config.Mount, error) {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) < 2 {
		return config.Mount{}, fmt.Errorf("volume requires source:target")
	}

	source := parts[0]
	target := parts[1]
	readOnly := len(parts) == 3 && parts[2] == "ro"

	if !filepath.IsAbs(source) {
		source = filepath.Join(baseDir, source)
		if abs, err := filepath.Abs(source); err == nil {
			source = abs
		}
	}

	return config.Mount{
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}, nil
}

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
