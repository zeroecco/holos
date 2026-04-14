package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"path/filepath"
	"regexp"
	"strings"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Manifest struct {
	APIVersion  string            `json:"api_version"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Replicas    int               `json:"replicas"`
	Image       string            `json:"image"`
	ImageFormat string            `json:"image_format"`
	VM          VMConfig          `json:"vm"`
	Network     NetworkConfig     `json:"network"`
	Ports       []PortForward     `json:"ports"`
	Mounts      []Mount           `json:"mounts"`
	CloudInit   CloudInit         `json:"cloud_init"`
	Labels          map[string]string      `json:"labels"`
	Devices         []Device               `json:"devices,omitempty"`
	InternalNetwork *InternalNetworkConfig `json:"internal_network,omitempty"`
	ExtraHosts      map[string]string      `json:"extra_hosts,omitempty"`
}

type VMConfig struct {
	VCPU      int      `json:"vcpu"`
	MemoryMB  int      `json:"memory_mb"`
	Machine   string   `json:"machine"`
	CPUModel  string   `json:"cpu_model"`
	Features  []string `json:"features"`
	UEFI      bool     `json:"uefi,omitempty"`
	ExtraArgs []string `json:"extra_args,omitempty"`
}

type Device struct {
	PCI    string `json:"pci,omitempty"`
	ROMFile string `json:"rom_file,omitempty"`
}

type NetworkConfig struct {
	Mode string `json:"mode"`
}

type InternalNetworkConfig struct {
	MulticastGroup string   `json:"multicast_group"`
	MulticastPort  int      `json:"multicast_port"`
	Subnet         string   `json:"subnet"`
	InstanceIPs    []string `json:"instance_ips"`
	BaseMAC        string   `json:"base_mac"`
	UserBaseMAC    string   `json:"user_base_mac"`
}

func (n *InternalNetworkConfig) InstanceMAC(index int) string {
	parts := strings.Split(n.BaseMAC, ":")
	if len(parts) != 6 {
		return n.BaseMAC
	}
	last, _ := strconv.ParseUint(parts[5], 16, 8)
	parts[5] = fmt.Sprintf("%02x", byte(last)+byte(index))
	return strings.Join(parts, ":")
}

func (n *InternalNetworkConfig) UserMAC(index int) string {
	parts := strings.Split(n.UserBaseMAC, ":")
	if len(parts) != 6 {
		return n.UserBaseMAC
	}
	last, _ := strconv.ParseUint(parts[5], 16, 8)
	parts[5] = fmt.Sprintf("%02x", byte(last)+byte(index))
	return strings.Join(parts, ":")
}

func (n *InternalNetworkConfig) InstanceIP(index int) string {
	if index < len(n.InstanceIPs) {
		return n.InstanceIPs[index]
	}
	return ""
}

type PortForward struct {
	Name      string `json:"name"`
	HostPort  int    `json:"host_port"`
	GuestPort int    `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type CloudInit struct {
	Hostname          string      `json:"hostname"`
	User              string      `json:"user"`
	SSHAuthorizedKeys []string    `json:"ssh_authorized_keys"`
	Packages          []string    `json:"packages"`
	BootCmd           []string    `json:"bootcmd"`
	RunCmd            []string    `json:"runcmd"`
	WriteFiles        []WriteFile `json:"write_files"`
}

type WriteFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
}

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
		m.APIVersion = "holosteric/v1alpha1"
	}
	if m.Kind == "" {
		m.Kind = "Service"
	}
	if m.Replicas == 0 {
		m.Replicas = 1
	}
	if m.ImageFormat == "" {
		m.ImageFormat = inferImageFormat(m.Image)
	}
	if m.VM.VCPU == 0 {
		m.VM.VCPU = 1
	}
	if m.VM.MemoryMB == 0 {
		m.VM.MemoryMB = 512
	}
	if m.VM.Machine == "" {
		m.VM.Machine = "q35"
	}
	if m.VM.CPUModel == "" {
		m.VM.CPUModel = "host"
	}
	if m.Network.Mode == "" {
		m.Network.Mode = "user"
	}
	if m.CloudInit.User == "" {
		m.CloudInit.User = "ubuntu"
	}
	for i := range m.Ports {
		if m.Ports[i].Protocol == "" {
			m.Ports[i].Protocol = "tcp"
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

func (m Manifest) SpecHash() (string, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal manifest for hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

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
