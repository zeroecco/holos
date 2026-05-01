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
var userNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var pciAddressPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)

const (
	DefaultReplicas           = 1
	DefaultVCPU               = 1
	DefaultMemoryMB           = 512
	DefaultMachine            = "q35"
	DefaultCPUModel           = "host"
	DefaultUser               = "ubuntu"
	DefaultProtocol           = "tcp"
	DefaultStopGracePeriodSec = 30

	DefaultHealthIntervalSec = 30
	DefaultHealthRetries     = 3
	DefaultHealthTimeoutSec  = 5

	// minVolumeSizeBytes is enforced in Validate so a corrupted or
	// hand-written manifest can't request a 0-byte volume that would
	// confuse qemu-img.
	minVolumeSizeBytes = 1 << 20 // 1 MiB
)

const (
	ImageOSSystemd = "systemd"
	ImageOSOpenRC  = "openrc"
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
	ImageOS            string                 `json:"image_os,omitempty"`
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
	Healthcheck        *HealthcheckConfig     `json:"healthcheck,omitempty"`
	// DependsOn is the resolved list of services this one must come
	// up after. Purely informational for the runtime (topological
	// ordering is already baked into Project.ServiceOrder), but the
	// reverse edge is what we use to decide which services need a
	// wait-for-healthy gate.
	DependsOn []string `json:"depends_on,omitempty"`
}

// HealthcheckConfig is the runtime-ready form of a compose healthcheck.
// All durations are expressed as whole seconds so the on-disk record is
// trivially inspectable with `jq`. Zero values mean "no healthcheck";
// callers that hold a nil *HealthcheckConfig skip probing entirely.
type HealthcheckConfig struct {
	// Test is the argv passed to `sh -c` (or a direct exec) inside
	// the VM. Never empty when the pointer is non-nil; Validate()
	// enforces this.
	Test []string `json:"test"`

	IntervalSec    int `json:"interval_sec"`
	Retries        int `json:"retries"`
	StartPeriodSec int `json:"start_period_sec,omitempty"`
	TimeoutSec     int `json:"timeout_sec,omitempty"`
}

// VMConfig specifies virtual hardware: CPU count, memory, root disk size,
// machine type, CPU model, UEFI boot, and arbitrary extra QEMU arguments.
type VMConfig struct {
	VCPU          int      `json:"vcpu"`
	MemoryMB      int      `json:"memory_mb"`
	DiskSizeBytes int64    `json:"disk_size_bytes,omitempty"`
	Machine       string   `json:"machine"`
	CPUModel      string   `json:"cpu_model"`
	Features      []string `json:"features"`
	UEFI          bool     `json:"uefi,omitempty"`
	ExtraArgs     []string `json:"extra_args,omitempty"`
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

// Mount attaches storage into a guest VM. Two flavours are supported:
//
//   - Kind "bind" (default): a host directory shared read/write or
//     read-only via 9p/virtfs. Source is an absolute host path.
//   - Kind "volume": a named qcow2 volume owned by holos. VolumeName
//     selects the backing file under state_dir/volumes/<project>/;
//     SizeBytes records the virtual size declared in compose so the
//     runtime can qemu-img create on first use.
//
// Target is always an in-guest absolute path.
type Mount struct {
	Kind       string `json:"kind,omitempty"`
	Source     string `json:"source,omitempty"`
	VolumeName string `json:"volume_name,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	Target     string `json:"target"`
	ReadOnly   bool   `json:"read_only"`
}

// Mount kind discriminators. Left as strings (not iota) so the on-disk
// JSON is self-documenting and forward-compatible with future kinds.
const (
	MountKindBind   = "bind"
	MountKindVolume = "volume"
)

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
	if m.Healthcheck != nil {
		if m.Healthcheck.IntervalSec == 0 {
			m.Healthcheck.IntervalSec = DefaultHealthIntervalSec
		}
		if m.Healthcheck.Retries == 0 {
			m.Healthcheck.Retries = DefaultHealthRetries
		}
		if m.Healthcheck.TimeoutSec == 0 {
			m.Healthcheck.TimeoutSec = DefaultHealthTimeoutSec
		}
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
	for i := range m.Mounts {
		if m.Mounts[i].Kind == "" {
			m.Mounts[i].Kind = MountKindBind
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
		// Only bind mounts resolve host paths; named volumes are
		// materialised by the runtime under state_dir/volumes/.
		if m.Mounts[i].Kind == MountKindVolume {
			continue
		}
		if m.Mounts[i].Source == "" {
			continue
		}
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
	if m.ImageOS != "" && m.ImageOS != ImageOSSystemd && m.ImageOS != ImageOSOpenRC {
		return fmt.Errorf("image_os must be one of %s or %s", ImageOSSystemd, ImageOSOpenRC)
	}
	if m.VM.VCPU < 1 {
		return fmt.Errorf("vm.vcpu must be >= 1")
	}
	if m.VM.MemoryMB < 128 {
		return fmt.Errorf("vm.memory_mb must be >= 128")
	}
	if m.VM.DiskSizeBytes != 0 && m.VM.DiskSizeBytes < minVolumeSizeBytes {
		return fmt.Errorf("vm.disk_size_bytes must be 0 or >= %d", minVolumeSizeBytes)
	}
	if m.Network.Mode != "user" {
		return fmt.Errorf("network.mode %q is unsupported; only user is implemented", m.Network.Mode)
	}
	if err := ValidateUserName(m.CloudInit.User); err != nil {
		return fmt.Errorf("cloud_init.user: %w", err)
	}
	for _, device := range m.Devices {
		if err := ValidatePCIAddress(device.PCI); err != nil {
			return fmt.Errorf("device pci %q: %w", device.PCI, err)
		}
	}
	if m.StopGracePeriodSec < 0 {
		return fmt.Errorf("stop_grace_period_sec must be >= 0")
	}
	if m.Healthcheck != nil {
		if len(m.Healthcheck.Test) == 0 {
			return fmt.Errorf("healthcheck.test is required")
		}
		if m.Healthcheck.IntervalSec < 1 {
			return fmt.Errorf("healthcheck.interval_sec must be >= 1")
		}
		if m.Healthcheck.Retries < 1 {
			return fmt.Errorf("healthcheck.retries must be >= 1")
		}
		if m.Healthcheck.TimeoutSec < 1 {
			return fmt.Errorf("healthcheck.timeout_sec must be >= 1")
		}
		if m.Healthcheck.StartPeriodSec < 0 {
			return fmt.Errorf("healthcheck.start_period_sec must be >= 0")
		}
	}
	// claimed tracks every static host port that any replica will
	// try to bind once the runtime has applied its per-replica
	// offset. Two compose entries that look independent on paper can
	// collide after the offset: `8080:80` and `8081:81` with
	// replicas: 2 both land on host 8081 for replica index 1, and
	// the second bind fails deep inside `holos up`. We model the
	// full cross product here so `holos validate` surfaces the
	// collision up front instead of mid-launch.
	//
	// Ephemeral ports (HostPort == 0) are allocated uniquely per
	// replica by the runtime and cannot collide, so we skip them.
	type claim struct{ baseHost, guest int }
	claimed := make(map[int]claim, len(m.Ports)*m.Replicas)
	for _, port := range m.Ports {
		if port.GuestPort < 1 || port.GuestPort > 65535 {
			return fmt.Errorf("guest port %d is out of range", port.GuestPort)
		}
		if port.HostPort < 0 || port.HostPort > 65535 {
			return fmt.Errorf("host port %d is out of range", port.HostPort)
		}
		if port.HostPort > 0 {
			top := port.HostPort + m.Replicas - 1
			if top > 65535 {
				return fmt.Errorf(
					"host port %d with replicas %d would overflow to %d (must be <= 65535)",
					port.HostPort, m.Replicas, top)
			}
			for r := 0; r < m.Replicas; r++ {
				host := port.HostPort + r
				if prev, dup := claimed[host]; dup {
					return fmt.Errorf(
						"host port %d is claimed by both mapping %d:%d and %d:%d at replica %d",
						host, prev.baseHost, prev.guest, port.HostPort, port.GuestPort, r)
				}
				claimed[host] = claim{baseHost: port.HostPort, guest: port.GuestPort}
			}
		}
		if port.Protocol != "tcp" {
			return fmt.Errorf("protocol %q is unsupported; only tcp is implemented", port.Protocol)
		}
	}
	for _, mount := range m.Mounts {
		if mount.Target == "" {
			return fmt.Errorf("mounts require target")
		}
		switch mount.Kind {
		case "", MountKindBind:
			if mount.Source == "" {
				return fmt.Errorf("bind mount %q requires source", mount.Target)
			}
		case MountKindVolume:
			if mount.VolumeName == "" {
				return fmt.Errorf("volume mount %q requires volume_name", mount.Target)
			}
			if mount.SizeBytes < minVolumeSizeBytes {
				return fmt.Errorf("volume %q size_bytes %d is below minimum %d",
					mount.VolumeName, mount.SizeBytes, minVolumeSizeBytes)
			}
		default:
			return fmt.Errorf("mount %q: unknown kind %q", mount.Target, mount.Kind)
		}
	}
	for _, file := range m.CloudInit.WriteFiles {
		if file.Path == "" {
			return fmt.Errorf("write_files entries require path")
		}
	}
	return nil
}

// ValidateUserName checks the guest account name holos asks cloud-init to
// create. Keep this deliberately aligned with the systemd User= validation:
// lowercase POSIX names are portable across the distro images holos targets and
// cannot inject shell, YAML, or unit-file syntax through later command paths.
func ValidateUserName(name string) error {
	if name == "" {
		return fmt.Errorf("user is empty")
	}
	if !userNamePattern.MatchString(name) {
		return fmt.Errorf("user %q must match %s (POSIX username, 1-32 lowercase/digits/underscore/hyphen, leading [a-z_])",
			name, userNamePattern.String())
	}
	return nil
}

// ValidatePCIAddress checks a canonical PCI BDF address:
// domain:bus:slot.function, with the function constrained to 0-7.
func ValidatePCIAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is empty")
	}
	if !pciAddressPattern.MatchString(addr) {
		return fmt.Errorf("must match 0000:01:00.0")
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
