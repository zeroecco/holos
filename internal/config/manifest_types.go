package config

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
	PreStopCommands    []string               `json:"pre_stop_commands,omitempty"`
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

	IntervalSec      int `json:"interval_sec"`
	Retries          int `json:"retries"`
	StartPeriodSec   int `json:"start_period_sec,omitempty"`
	StartIntervalSec int `json:"start_interval_sec,omitempty"`
	TimeoutSec       int `json:"timeout_sec,omitempty"`
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
	MulticastGroup string                   `json:"multicast_group"`
	MulticastPort  int                      `json:"multicast_port"`
	Subnet         string                   `json:"subnet"`
	InstanceIPs    []string                 `json:"instance_ips"`
	BaseMAC        string                   `json:"base_mac"`
	UserBaseMAC    string                   `json:"user_base_mac"`
	DNSSearch      []string                 `json:"dns_search,omitempty"`
	Segments       []InternalNetworkSegment `json:"segments,omitempty"`
}

// InternalNetworkSegment is an additional named internal L2 segment attached
// to a VM alongside the primary internal network.
type InternalNetworkSegment struct {
	Name           string   `json:"name"`
	MulticastGroup string   `json:"multicast_group"`
	MulticastPort  int      `json:"multicast_port"`
	Subnet         string   `json:"subnet"`
	InstanceIPs    []string `json:"instance_ips"`
	BaseMAC        string   `json:"base_mac"`
}

// InstanceMAC returns the internal NIC MAC address for the given replica index.
func (n *InternalNetworkConfig) InstanceMAC(index int) string {
	return offsetMAC(n.BaseMAC, index)
}

// UserMAC returns the user-mode NIC MAC address for the given replica index.
func (n *InternalNetworkConfig) UserMAC(index int) string {
	return offsetMAC(n.UserBaseMAC, index)
}

// InstanceIP returns the static IP for the given replica index, or "" if
// the index is out of range.
func (n *InternalNetworkConfig) InstanceIP(index int) string {
	if index < len(n.InstanceIPs) {
		return n.InstanceIPs[index]
	}
	return ""
}

// SegmentMAC returns the segment NIC MAC address for the given replica index.
func (s InternalNetworkSegment) SegmentMAC(index int) string {
	return offsetMAC(s.BaseMAC, index)
}

// SegmentIP returns the static segment IP for the given replica index.
func (s InternalNetworkSegment) SegmentIP(index int) string {
	if index < len(s.InstanceIPs) {
		return s.InstanceIPs[index]
	}
	return ""
}

// PortForward maps a host TCP/UDP port/address to a guest TCP/UDP port/address.
type PortForward struct {
	Name      string `json:"name"`
	HostAddr  string `json:"host_addr,omitempty"`
	HostPort  int    `json:"host_port"`
	GuestAddr string `json:"guest_addr,omitempty"`
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
