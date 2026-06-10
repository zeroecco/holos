package compose

import "github.com/zeroecco/holos/internal/config"

// File is the user-facing YAML compose format.
type File struct {
	Version         string             `yaml:"version,omitempty"`
	Name            string             `yaml:"name"`
	Include         IncludeFiles       `yaml:"include,omitempty"`
	Services        map[string]Service `yaml:"services"`
	Volumes         map[string]Volume  `yaml:"volumes,omitempty"`
	Networks        map[string]Network `yaml:"networks,omitempty"`
	Configs         map[string]Config  `yaml:"configs,omitempty"`
	Secrets         map[string]Secret  `yaml:"secrets,omitempty"`
	Models          map[string]Model   `yaml:"models,omitempty"`
	baseDir         string
	serviceBaseDirs map[string]string
	imageResolver   composeImageResolver
}

// Service is a single VM definition within the compose file.
type Service struct {
	Image             string            `yaml:"image"`
	ImageFormat       string            `yaml:"image_format,omitempty"`
	ImageOS           string            `yaml:"image_os,omitempty"`
	Build             ComposeBuild      `yaml:"build,omitempty"`
	Dockerfile        string            `yaml:"dockerfile,omitempty"`
	Command           ComposeCommand    `yaml:"command,omitempty"`
	Entrypoint        ComposeCommand    `yaml:"entrypoint,omitempty"`
	WorkingDir        string            `yaml:"working_dir,omitempty"`
	User              string            `yaml:"user,omitempty"`
	ContainerName     string            `yaml:"container_name,omitempty"`
	Platform          string            `yaml:"platform,omitempty"`
	PullPolicy        string            `yaml:"pull_policy,omitempty"`
	PullRefreshAfter  string            `yaml:"pull_refresh_after,omitempty"`
	Profiles          []string          `yaml:"profiles,omitempty"`
	Attach            any               `yaml:"attach,omitempty"`
	Restart           string            `yaml:"restart,omitempty"`
	StopSignal        string            `yaml:"stop_signal,omitempty"`
	Deploy            Deploy            `yaml:"deploy,omitempty"`
	Replicas          int               `yaml:"replicas,omitempty"`
	Scale             any               `yaml:"scale,omitempty"`
	Hostname          string            `yaml:"hostname,omitempty"`
	DomainName        string            `yaml:"domainname,omitempty"`
	CPUs              any               `yaml:"cpus,omitempty"`
	MemLimit          string            `yaml:"mem_limit,omitempty"`
	MemReservation    string            `yaml:"mem_reservation,omitempty"`
	MemSwappiness     any               `yaml:"mem_swappiness,omitempty"`
	MemSwapLimit      any               `yaml:"memswap_limit,omitempty"`
	ShmSize           any               `yaml:"shm_size,omitempty"`
	Init              any               `yaml:"init,omitempty"`
	OOMKillDisable    any               `yaml:"oom_kill_disable,omitempty"`
	OOMScoreAdj       any               `yaml:"oom_score_adj,omitempty"`
	PidsLimit         any               `yaml:"pids_limit,omitempty"`
	Privileged        any               `yaml:"privileged,omitempty"`
	ReadOnly          any               `yaml:"read_only,omitempty"`
	TTY               any               `yaml:"tty,omitempty"`
	StdinOpen         any               `yaml:"stdin_open,omitempty"`
	CapAdd            StringList        `yaml:"cap_add,omitempty"`
	CapDrop           StringList        `yaml:"cap_drop,omitempty"`
	Cgroup            string            `yaml:"cgroup,omitempty"`
	CgroupParent      string            `yaml:"cgroup_parent,omitempty"`
	CPUCount          int               `yaml:"cpu_count,omitempty"`
	CPUPercent        int               `yaml:"cpu_percent,omitempty"`
	CPUPeriod         any               `yaml:"cpu_period,omitempty"`
	CPUQuota          any               `yaml:"cpu_quota,omitempty"`
	CPURTPeriod       any               `yaml:"cpu_rt_period,omitempty"`
	CPURTRuntime      any               `yaml:"cpu_rt_runtime,omitempty"`
	CPUShares         any               `yaml:"cpu_shares,omitempty"`
	CPUSet            string            `yaml:"cpuset,omitempty"`
	CredentialSpec    map[string]string `yaml:"credential_spec,omitempty"`
	Isolation         string            `yaml:"isolation,omitempty"`
	IPC               string            `yaml:"ipc,omitempty"`
	PID               string            `yaml:"pid,omitempty"`
	Runtime           string            `yaml:"runtime,omitempty"`
	SecurityOpt       StringList        `yaml:"security_opt,omitempty"`
	StorageOpt        map[string]string `yaml:"storage_opt,omitempty"`
	Sysctls           ComposeLabels     `yaml:"sysctls,omitempty"`
	Tmpfs             StringList        `yaml:"tmpfs,omitempty"`
	Ulimits           map[string]any    `yaml:"ulimits,omitempty"`
	UTS               string            `yaml:"uts,omitempty"`
	UsernsMode        string            `yaml:"userns_mode,omitempty"`
	BlkioConfig       BlkioConfig       `yaml:"blkio_config,omitempty"`
	DeviceCgroupRules StringList        `yaml:"device_cgroup_rules,omitempty"`
	DeviceReadBPS     []ThrottleDevice  `yaml:"device_read_bps,omitempty"`
	DeviceReadIOPS    []ThrottleDevice  `yaml:"device_read_iops,omitempty"`
	DeviceWriteBPS    []ThrottleDevice  `yaml:"device_write_bps,omitempty"`
	DeviceWriteIOPS   []ThrottleDevice  `yaml:"device_write_iops,omitempty"`
	VM                VM                `yaml:"vm,omitempty"`
	Ports             []ComposePort     `yaml:"ports,omitempty"`
	Volumes           []ComposeVolume   `yaml:"volumes,omitempty"`
	Devices           []ComposeDevice   `yaml:"devices,omitempty"`
	DependsOn         DependsOn         `yaml:"depends_on,omitempty"`
	Labels            ComposeLabels     `yaml:"labels,omitempty"`
	LabelFile         StringList        `yaml:"label_file,omitempty"`
	Annotations       ComposeLabels     `yaml:"annotations,omitempty"`
	ExtraHosts        ExtraHosts        `yaml:"extra_hosts,omitempty"`
	DNS               StringList        `yaml:"dns,omitempty"`
	DNSOpt            StringList        `yaml:"dns_opt,omitempty"`
	DNSSearch         StringList        `yaml:"dns_search,omitempty"`
	EnvFile           EnvFiles          `yaml:"env_file,omitempty"`
	Environment       Environment       `yaml:"environment,omitempty"`
	Expose            StringList        `yaml:"expose,omitempty"`
	ExternalLinks     StringList        `yaml:"external_links,omitempty"`
	GroupAdd          StringList        `yaml:"group_add,omitempty"`
	Links             StringList        `yaml:"links,omitempty"`
	NetworkMode       string            `yaml:"network_mode,omitempty"`
	Networks          ServiceNetworks   `yaml:"networks,omitempty"`
	Configs           ServiceResources  `yaml:"configs,omitempty"`
	Secrets           ServiceResources  `yaml:"secrets,omitempty"`
	Models            ServiceModels     `yaml:"models,omitempty"`
	Develop           Develop           `yaml:"develop,omitempty"`
	Extends           Extends           `yaml:"extends,omitempty"`
	GPUs              GPUs              `yaml:"gpus,omitempty"`
	Logging           Logging           `yaml:"logging,omitempty"`
	MacAddress        string            `yaml:"mac_address,omitempty"`
	PostStart         []LifecycleHook   `yaml:"post_start,omitempty"`
	PreStop           []LifecycleHook   `yaml:"pre_stop,omitempty"`
	Provider          Provider          `yaml:"provider,omitempty"`
	UseAPISocket      bool              `yaml:"use_api_socket,omitempty"`
	VolumesFrom       StringList        `yaml:"volumes_from,omitempty"`
	CloudInit         CloudInit         `yaml:"cloud_init,omitempty"`
	StopGracePeriod   string            `yaml:"stop_grace_period,omitempty"`
	Healthcheck       *Healthcheck      `yaml:"healthcheck,omitempty"`
}

// VM configures the virtual hardware for a service.
type VM struct {
	VCPU     int `yaml:"vcpu,omitempty"`
	MemoryMB int `yaml:"memory_mb,omitempty"`
	// DiskSize is the requested virtual size of the writable root overlay.
	// Empty keeps qemu-img's default backing-image size.
	DiskSize string `yaml:"disk_size,omitempty"`
	Machine  string `yaml:"machine,omitempty"`
	CPUModel string `yaml:"cpu_model,omitempty"`
	UEFI     bool   `yaml:"uefi,omitempty"`

	ExtraArgs []string `yaml:"extra_args,omitempty"`
}

// Volume configures a top-level named volume. Named volumes are
// qcow2-backed block devices that persist across `holos down`. They
// live under state_dir/volumes/<project>/<name>.qcow2 and are symlinked
// into each instance's workdir so teardown only removes the symlink.
type Volume struct {
	// Size is a human-friendly capacity like "10G", "500M", "1T".
	// Empty defaults to 10 GiB. The value is the VIRTUAL size of the
	// qcow2; on-disk usage grows sparsely with actual writes.
	Size       string            `yaml:"size,omitempty"`
	Name       string            `yaml:"name,omitempty"`
	Driver     string            `yaml:"driver,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
	External   any               `yaml:"external,omitempty"`
	Labels     ComposeLabels     `yaml:"labels,omitempty"`
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
	Name       string
	SizeBytes  int64
	SourcePath string
}

// NetworkPlan describes the internal network assigned to a project.
type NetworkPlan struct {
	MulticastGroup string
	MulticastPort  int
	Subnet         string
	Hosts          map[string]string
	Segments       map[string]NetworkSegmentPlan
}

// NetworkSegmentPlan is a named internal L2 segment derived from Compose
// network declarations and service attachments.
type NetworkSegmentPlan struct {
	Name           string
	MulticastGroup string
	MulticastPort  int
	Subnet         string
	Hosts          map[string]string
	Backend        string
	BridgeName     string
}
