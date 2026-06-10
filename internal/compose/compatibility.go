package compose

type BlkioConfig struct {
	Weight          any              `yaml:"weight,omitempty"`
	WeightDevice    []WeightedDevice `yaml:"weight_device,omitempty"`
	DeviceReadBPS   []ThrottleDevice `yaml:"device_read_bps,omitempty"`
	DeviceReadIOPS  []ThrottleDevice `yaml:"device_read_iops,omitempty"`
	DeviceWriteBPS  []ThrottleDevice `yaml:"device_write_bps,omitempty"`
	DeviceWriteIOPS []ThrottleDevice `yaml:"device_write_iops,omitempty"`
}

type WeightedDevice struct {
	Path   string `yaml:"path,omitempty"`
	Weight any    `yaml:"weight,omitempty"`
}

type ThrottleDevice struct {
	Path string `yaml:"path,omitempty"`
	Rate any    `yaml:"rate,omitempty"`
}

type Develop struct {
	Watch []DevelopWatch `yaml:"watch,omitempty"`
}

type DevelopWatch struct {
	Path        string      `yaml:"path,omitempty"`
	Action      string      `yaml:"action,omitempty"`
	Target      string      `yaml:"target,omitempty"`
	Ignore      StringList  `yaml:"ignore,omitempty"`
	Include     StringList  `yaml:"include,omitempty"`
	InitialSync bool        `yaml:"initial_sync,omitempty"`
	Exec        DevelopExec `yaml:"exec,omitempty"`
}

type DevelopExec struct {
	Command     ComposeCommand `yaml:"command,omitempty"`
	User        string         `yaml:"user,omitempty"`
	Privileged  any            `yaml:"privileged,omitempty"`
	WorkingDir  string         `yaml:"working_dir,omitempty"`
	Environment Environment    `yaml:"environment,omitempty"`
}

type Extends struct {
	File    string `yaml:"file,omitempty"`
	Service string `yaml:"service,omitempty"`
}

type Logging struct {
	Driver  string         `yaml:"driver,omitempty"`
	Options map[string]any `yaml:"options,omitempty"`
}

type LifecycleHook struct {
	Command     ComposeCommand `yaml:"command,omitempty"`
	User        string         `yaml:"user,omitempty"`
	Privileged  any            `yaml:"privileged,omitempty"`
	WorkingDir  string         `yaml:"working_dir,omitempty"`
	Environment Environment    `yaml:"environment,omitempty"`
}

type Provider struct {
	Type    string         `yaml:"type,omitempty"`
	Options map[string]any `yaml:"options,omitempty"`
}
