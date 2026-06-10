package compose

// Deploy accepts Docker Compose deploy syntax. Holos maps replicas and simple
// CPU/memory limits into its VM model; the remaining swarm-oriented fields are
// retained as compatibility metadata.
type Deploy struct {
	Mode           string              `yaml:"mode,omitempty"`
	Replicas       int                 `yaml:"replicas,omitempty"`
	EndpointMode   string              `yaml:"endpoint_mode,omitempty"`
	Labels         ComposeLabels       `yaml:"labels,omitempty"`
	Resources      DeployResources     `yaml:"resources,omitempty"`
	RestartPolicy  DeployRestartPolicy `yaml:"restart_policy,omitempty"`
	Placement      DeployPlacement     `yaml:"placement,omitempty"`
	UpdateConfig   DeployUpdateConfig  `yaml:"update_config,omitempty"`
	RollbackConfig DeployUpdateConfig  `yaml:"rollback_config,omitempty"`
}

type DeployResources struct {
	Limits       DeployResource `yaml:"limits,omitempty"`
	Reservations DeployResource `yaml:"reservations,omitempty"`
}

type DeployResource struct {
	CPUs             string                `yaml:"cpus,omitempty"`
	Memory           string                `yaml:"memory,omitempty"`
	Pids             any                   `yaml:"pids,omitempty"`
	GenericResources []any                 `yaml:"generic_resources,omitempty"`
	Devices          []DeployDeviceRequest `yaml:"devices,omitempty"`
}

type DeployDeviceRequest struct {
	Capabilities StringList       `yaml:"capabilities,omitempty"`
	Driver       string           `yaml:"driver,omitempty"`
	Count        any              `yaml:"count,omitempty"`
	DeviceIDs    StringList       `yaml:"device_ids,omitempty"`
	Options      ComposeListOrMap `yaml:"options,omitempty"`
}

type DeployRestartPolicy struct {
	Condition   string `yaml:"condition,omitempty"`
	Delay       string `yaml:"delay,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
	Window      string `yaml:"window,omitempty"`
}

type DeployPlacement struct {
	Constraints        []string `yaml:"constraints,omitempty"`
	Preferences        []any    `yaml:"preferences,omitempty"`
	MaxReplicasPerNode int      `yaml:"max_replicas_per_node,omitempty"`
}

type DeployUpdateConfig struct {
	Parallelism     int     `yaml:"parallelism,omitempty"`
	Delay           string  `yaml:"delay,omitempty"`
	FailureAction   string  `yaml:"failure_action,omitempty"`
	Monitor         string  `yaml:"monitor,omitempty"`
	MaxFailureRatio float64 `yaml:"max_failure_ratio,omitempty"`
	Order           string  `yaml:"order,omitempty"`
}
