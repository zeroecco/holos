package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

const (
	DefaultReplicas           = 1
	DefaultVCPU               = 1
	DefaultMemoryMB           = 512
	DefaultMachine            = "q35"
	DefaultCPUModel           = "host"
	DefaultUser               = "ubuntu"
	DefaultProtocol           = "tcp"
	DefaultStopGracePeriodSec = 30
)

// Manifest is the fully resolved description of a single service, consumed
// by the runtime and qemu packages to launch VM instances.
type Manifest struct {
	APIVersion         string                 `json:"api_version"`
	Kind               string                 `json:"kind"`
	Name               string                 `json:"name"`
	Replicas           int                    `json:"replicas"`
	Image              string                 `json:"image"`
	ImageFormat        string                 `json:"image_format"`
	VM                 VMConfig               `json:"vm"`
	Network            NetworkConfig          `json:"network"`
	Ports              []PortForward          `json:"ports"`
	Mounts             []Mount                `json:"mounts"`
	CloudInit          CloudInit              `json:"cloud_init"`
	Labels             map[string]string      `json:"labels"`
	Devices            []Device               `json:"devices,omitempty"`
	InternalNetwork    *InternalNetworkConfig `json:"internal_network,omitempty"`
	ExtraHosts         map[string]string      `json:"extra_hosts,omitempty"`
	StopGracePeriodSec int                    `json:"stop_grace_period_sec,omitempty"`
}

// VMConfig specifies virtual hardware: CPU count, memory, machine type,
// CPU model, UEFI boot, and arbitrary extra QEMU arguments.
type VMConfig struct {
	VCPU      int      `json:"vcpu"`
	MemoryMB  int      `json:"memory_mb"`
	Machine   string   `json:"machine"`
	CPUModel  string   `json:"cpu_model"`
	Features  []string `json:"features"`
	UEFI      bool     `json:"uefi,omitempty"`
	ExtraArgs []string `json:"extra_args,omitempty"`
}

// Device is a PCI device for VFIO passthrough.
type Device struct {
	PCI     string `json:"pci,omitempty"`
	ROMFile string `json:"rom_file,omitempty"`
}

// NetworkConfig selects the QEMU networking mode (currently only "user").
type NetworkConfig struct {
	Mode string `json:"mode"`
}

// InternalNetworkConfig describes the socket-multicast inter-VM network
// assigned by the compose resolver: multicast group/port, subnet, per-replica
// IPs, and base MAC addresses for both the internal and user-mode NICs.
type InternalNetworkConfig struct {
	MulticastGroup string   `json:"multicast_group"`
	MulticastPort  int      `json:"multicast_port"`
	Subnet         string   `json:"subnet"`
	InstanceIPs    []string `json:"instance_ips"`
	BaseMAC        string   `json:"base_mac"`
	UserBaseMAC    string   `json:"user_base_mac"`
}

// InstanceMAC returns the internal NIC MAC address for the given replica index.
func (n *InternalNetworkConfig) InstanceMAC(index int) string {
	return offsetMAC(n.BaseMAC, index)
}

// UserMAC returns the user-mode NIC MAC address for the given replica index.
func (n *InternalNetworkConfig) UserMAC(index int) string {
	return offsetMAC(n.UserBaseMAC, index)
}

func offsetMAC(base string, index int) string {
	parts := strings.Split(base, ":")
	if len(parts) != 6 {
		return base
	}
	last, _ := strconv.ParseUint(parts[5], 16, 8)
	parts[5] = fmt.Sprintf("%02x", byte(last)+byte(index))
	return strings.Join(parts, ":")
}

// InstanceIP returns the static IP for the given replica index, or "" if
// the index is out of range.
func (n *InternalNetworkConfig) InstanceIP(index int) string {
	if index < len(n.InstanceIPs) {
		return n.InstanceIPs[index]
	}
	return ""
}

// PortForward maps a host TCP port to a guest TCP port.
type PortForward struct {
	Name      string `json:"name"`
	HostPort  int    `json:"host_port"`
	GuestPort int    `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

// Mount describes a host directory shared into the VM via 9p/virtfs.
type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// CloudInit holds the cloud-init parameters written into the NoCloud seed.
type CloudInit struct {
	Hostname          string      `json:"hostname"`
	User              string      `json:"user"`
	SSHAuthorizedKeys []string    `json:"ssh_authorized_keys"`
	Packages          []string    `json:"packages"`
	BootCmd           []string    `json:"bootcmd"`
	RunCmd            []string    `json:"runcmd"`
	WriteFiles        []WriteFile `json:"write_files"`
}

// WriteFile is a file to create inside the VM during cloud-init.
type WriteFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
}

// LoadManifest reads a JSON manifest file, applies defaults, resolves
// relative paths against the manifest's directory, and validates the result.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	manifest.applyDefaults()
	if err := manifest.resolvePaths(filepath.Dir(path)); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m *Manifest) applyDefaults() {
	if m.APIVersion == "" {
		m.APIVersion = "holos/v1alpha1"
	}
	if m.Kind == "" {
		m.Kind = "Service"
	}
	if m.Replicas == 0 {
		m.Replicas = DefaultReplicas
	}
	if m.ImageFormat == "" {
		m.ImageFormat = inferImageFormat(m.Image)
	}
	if m.VM.VCPU == 0 {
		m.VM.VCPU = DefaultVCPU
	}
	if m.VM.MemoryMB == 0 {
		m.VM.MemoryMB = DefaultMemoryMB
	}
	if m.VM.Machine == "" {
		m.VM.Machine = DefaultMachine
	}
	if m.VM.CPUModel == "" {
		m.VM.CPUModel = DefaultCPUModel
	}
	if m.Network.Mode == "" {
		m.Network.Mode = "user"
	}
	if m.CloudInit.User == "" {
		m.CloudInit.User = DefaultUser
	}
	if m.StopGracePeriodSec == 0 {
		m.StopGracePeriodSec = DefaultStopGracePeriodSec
	}
	for i := range m.Ports {
		if m.Ports[i].Protocol == "" {
			m.Ports[i].Protocol = DefaultProtocol
		}
	}
	for i := range m.CloudInit.WriteFiles {
		if m.CloudInit.WriteFiles[i].Permissions == "" {
			m.CloudInit.WriteFiles[i].Permissions = "0644"
		}
		if m.CloudInit.WriteFiles[i].Owner == "" {
			m.CloudInit.WriteFiles[i].Owner = "root:root"
		}
	}
}

func (m *Manifest) resolvePaths(baseDir string) error {
	if m.Image == "" {
		return nil
	}

	image, err := resolvePath(baseDir, m.Image)
	if err != nil {
		return fmt.Errorf("resolve image path: %w", err)
	}
	m.Image = image

	for i := range m.Mounts {
		source, err := resolvePath(baseDir, m.Mounts[i].Source)
		if err != nil {
			return fmt.Errorf("resolve mount %q: %w", m.Mounts[i].Source, err)
		}
		m.Mounts[i].Source = source
	}
	return nil
}

// Validate checks that all manifest fields are within acceptable ranges and
// formats. Returns the first validation error encountered, or nil.
func (m Manifest) Validate() error {
	if !serviceNamePattern.MatchString(m.Name) {
		return fmt.Errorf("name %q must match %s", m.Name, serviceNamePattern.String())
	}
	if m.Replicas < 1 {
		return fmt.Errorf("replicas must be >= 1")
	}
	if m.Image == "" {
		return fmt.Errorf("image is required")
	}
	if m.ImageFormat != "qcow2" && m.ImageFormat != "raw" {
		return fmt.Errorf("image_format must be one of qcow2 or raw")
	}
	if m.VM.VCPU < 1 {
		return fmt.Errorf("vm.vcpu must be >= 1")
	}
	if m.VM.MemoryMB < 128 {
		return fmt.Errorf("vm.memory_mb must be >= 128")
	}
	if m.Network.Mode != "user" {
		return fmt.Errorf("network.mode %q is unsupported; only user is implemented", m.Network.Mode)
	}
	if m.StopGracePeriodSec < 0 {
		return fmt.Errorf("stop_grace_period_sec must be >= 0")
	}
	for _, port := range m.Ports {
		if port.GuestPort < 1 || port.GuestPort > 65535 {
			return fmt.Errorf("guest port %d is out of range", port.GuestPort)
		}
		if port.HostPort < 0 || port.HostPort > 65535 {
			return fmt.Errorf("host port %d is out of range", port.HostPort)
		}
		if port.Protocol != "tcp" {
			return fmt.Errorf("protocol %q is unsupported; only tcp is implemented", port.Protocol)
		}
	}
	for _, mount := range m.Mounts {
		if mount.Source == "" || mount.Target == "" {
			return fmt.Errorf("mounts require source and target")
		}
	}
	for _, file := range m.CloudInit.WriteFiles {
		if file.Path == "" {
			return fmt.Errorf("write_files entries require path")
		}
	}
	return nil
}

// SpecHash returns the full hex-encoded SHA-256 of the JSON-marshaled manifest.
func (m Manifest) SpecHash() (string, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal manifest for hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// InstanceName returns the name for the replica at the given index
// (e.g. "web-0", "web-1").
func (m Manifest) InstanceName(index int) string {
	return fmt.Sprintf("%s-%d", m.Name, index)
}

func inferImageFormat(path string) string {
	switch filepath.Ext(path) {
	case ".raw", ".img":
		return "raw"
	default:
		return "qcow2"
	}
}

func resolvePath(baseDir, value string) (string, error) {
	if filepath.IsAbs(value) {
		return value, nil
	}
	absolute, err := filepath.Abs(filepath.Join(baseDir, value))
	if err != nil {
		return "", err
	}
	return absolute, nil
}
