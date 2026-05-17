package compose

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
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

// ValidateName enforces that a project, service, or volume name
// matches the DNS-label pattern used throughout holos.
//
// This is the single source of truth for "is this string safe to
// embed in a filesystem path or a systemd unit filename". The CLI
// funnels every user-supplied name (from `-f`, from positional
// arguments to `down`, `console`, `exec`, `logs`, and from
// `--name` on install/uninstall) through this helper so a value
// like "../../../etc/passwd" cannot be turned into a path like
// <state-dir>/projects/../../../etc/passwd.json or
// /etc/systemd/system/holos-../../etc/passwd.service.
//
// The pattern allows:
//   - 1 to 63 characters (DNS-label maximum)
//   - lowercase letters, digits, and hyphens
//   - first and last characters are alphanumeric
//
// That ruleset rejects path separators, path traversals (`..`),
// whitespace, control characters, shell metacharacters, and
// uppercase (which systemd treats case-insensitively on some
// filesystems, confusing `holos ps` output).
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name %q must match %s", name, namePattern.String())
	}
	return nil
}

// maxReplicas is a soft cap on `replicas:` for a single service to
// catch typos at parse time. It is intentionally larger than the
// per-project total so a single-service stack can use the full
// subnet and the error messages stay specific ("replicas 1000000
// exceeds maximum of 256" vs a surprise project-wide reject).
const maxReplicas = 256

// maxProjectInstances is the number of usable host addresses in
// subnetCIDR. The allocator starts at .2 and must stop at .254 to
// avoid producing nonsense octets like 10.10.0.270; reserving .1 for
// the gateway placeholder and .255 as the broadcast address leaves
// 253 addresses for VMs. This is the SUM of replicas across every
// service in a project, not a per-service limit.
const (
	maxProjectInstances = 253
	subnetCIDR          = "10.10.0.0/24"
)

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

// ComposeCommand accepts Docker Compose command/entrypoint string, list, null,
// and empty forms.
type ComposeCommand struct {
	Args   []string
	Set    bool
	Scalar bool
}

// ComposeBuild accepts Docker Compose build string and mapping syntax. Holos
// translates context/dockerfile into the existing Dockerfile provisioning path;
// other build keys are retained as compatibility metadata.
type ComposeBuild struct {
	Context            string           `yaml:"context,omitempty"`
	Dockerfile         string           `yaml:"dockerfile,omitempty"`
	DockerfileInline   string           `yaml:"dockerfile_inline,omitempty"`
	Args               Environment      `yaml:"args,omitempty"`
	AdditionalContexts ComposeStringMap `yaml:"additional_contexts,omitempty"`
	CacheFrom          StringList       `yaml:"cache_from,omitempty"`
	CacheTo            StringList       `yaml:"cache_to,omitempty"`
	Entitlements       StringList       `yaml:"entitlements,omitempty"`
	ExtraHosts         ExtraHosts       `yaml:"extra_hosts,omitempty"`
	Isolation          string           `yaml:"isolation,omitempty"`
	Labels             ComposeLabels    `yaml:"labels,omitempty"`
	Network            string           `yaml:"network,omitempty"`
	NoCache            any              `yaml:"no_cache,omitempty"`
	Pull               any              `yaml:"pull,omitempty"`
	Provenance         any              `yaml:"provenance,omitempty"`
	SBOM               any              `yaml:"sbom,omitempty"`
	Secrets            ServiceResources `yaml:"secrets,omitempty"`
	ShmSize            any              `yaml:"shm_size,omitempty"`
	SSH                ComposeStringMap `yaml:"ssh,omitempty"`
	Tags               StringList       `yaml:"tags,omitempty"`
	Target             string           `yaml:"target,omitempty"`
	Ulimits            map[string]any   `yaml:"ulimits,omitempty"`
	Platforms          StringList       `yaml:"platforms,omitempty"`
	Privileged         any              `yaml:"privileged,omitempty"`
}

func (b *ComposeBuild) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil
		}
		b.Context = node.Value
		return nil
	case yaml.MappingNode:
		allowed := map[string]struct{}{
			"context": {}, "dockerfile": {}, "dockerfile_inline": {}, "args": {},
			"additional_contexts": {}, "cache_from": {}, "cache_to": {},
			"entitlements": {}, "extra_hosts": {}, "isolation": {}, "labels": {},
			"network": {}, "no_cache": {}, "pull": {}, "provenance": {},
			"sbom": {}, "secrets": {}, "shm_size": {}, "ssh": {}, "tags": {},
			"target": {}, "ulimits": {}, "platforms": {}, "privileged": {},
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if _, ok := allowed[key]; !ok {
				return fmt.Errorf("line %d: field %s not found in type compose.ComposeBuild", node.Content[i].Line, key)
			}
		}
		type rawBuild ComposeBuild
		return node.Decode((*rawBuild)(b))
	default:
		return fmt.Errorf("line %d: build must be a string or mapping", node.Line)
	}
}

func (b ComposeBuild) isSet() bool {
	return b.Context != "" || b.Dockerfile != "" || b.DockerfileInline != ""
}

func (b ComposeBuild) dockerfilePath(baseDir string) (path string, contextDir string, ok bool, err error) {
	if !b.isSet() {
		return "", "", false, nil
	}
	contextDir = b.Context
	if contextDir == "" {
		contextDir = "."
	}
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(baseDir, contextDir)
	}
	if strings.TrimSpace(b.DockerfileInline) != "" {
		return "", contextDir, true, nil
	}
	path = b.Dockerfile
	if path == "" {
		path = "Dockerfile"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(contextDir, path)
	}
	return path, contextDir, true, nil
}

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

type GPUs struct {
	All      bool
	Requests []GPURequest
}

type GPURequest struct {
	Driver       string           `yaml:"driver,omitempty"`
	Count        any              `yaml:"count,omitempty"`
	Capabilities StringList       `yaml:"capabilities,omitempty"`
	DeviceIDs    StringList       `yaml:"device_ids,omitempty"`
	Options      ComposeListOrMap `yaml:"options,omitempty"`
}

func (g *GPUs) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "all" {
			g.All = true
			return nil
		}
		return fmt.Errorf("line %d: gpus scalar must be \"all\"", node.Line)
	case yaml.SequenceNode:
		var requests []GPURequest
		if err := node.Decode(&requests); err != nil {
			return err
		}
		g.Requests = requests
		return nil
	default:
		return fmt.Errorf("line %d: gpus must be \"all\" or a list of requests", node.Line)
	}
}

func (g GPUs) MarshalYAML() (any, error) {
	if g.All {
		return "all", nil
	}
	return g.Requests, nil
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

type Model struct {
	Name         string     `yaml:"name,omitempty"`
	Model        string     `yaml:"model,omitempty"`
	ContextSize  any        `yaml:"context_size,omitempty"`
	Runtime      string     `yaml:"runtime,omitempty"`
	RuntimeFlags StringList `yaml:"runtime_flags,omitempty"`
}

type ServiceModels map[string]ServiceModel

type ServiceModel struct {
	EndpointVar string `yaml:"endpoint_var,omitempty"`
	ModelVar    string `yaml:"model_var,omitempty"`
}

func (m *ServiceModels) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make(ServiceModels, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: model list entries must be scalar values", item.Line)
			}
			out[item.Value] = ServiceModel{}
		}
		*m = out
		return nil
	case yaml.MappingNode:
		out := make(ServiceModels, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			value := node.Content[i+1]
			if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
				out[name] = ServiceModel{}
				continue
			}
			var model ServiceModel
			if err := value.Decode(&model); err != nil {
				return err
			}
			out[name] = model
		}
		*m = out
		return nil
	default:
		return fmt.Errorf("line %d: models must be a list or mapping", node.Line)
	}
}

func (c *ComposeCommand) UnmarshalYAML(node *yaml.Node) error {
	c.Set = true
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			c.Args = nil
			return nil
		}
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		if s == "" {
			c.Args = nil
		} else {
			c.Args = []string{s}
			c.Scalar = true
		}
		return nil
	case yaml.SequenceNode:
		var args []string
		if err := node.Decode(&args); err != nil {
			return err
		}
		c.Args = args
		return nil
	default:
		return fmt.Errorf("line %d: command values must be a string, list, or null", node.Line)
	}
}

// StringList accepts Docker Compose fields that allow either a scalar string
// or a list of scalar values.
type StringList []string

func (l *StringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			*l = nil
			return nil
		}
		*l = StringList{node.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: list entries must be scalar values", item.Line)
			}
			if item.Tag == "!!null" {
				continue
			}
			out = append(out, item.Value)
		}
		*l = StringList(out)
		return nil
	default:
		return fmt.Errorf("line %d: value must be a string or list", node.Line)
	}
}

// ComposeStringMap accepts Compose fields that can be declared either as a
// mapping or as KEY=VALUE list entries.
type ComposeStringMap map[string]string

func (m *ComposeStringMap) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[key] = value
		}
		*m = ComposeStringMap(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			key, value, ok := strings.Cut(raw, "=")
			if !ok {
				return fmt.Errorf("line %d: entries must use KEY=VALUE syntax", item.Line)
			}
			out[key] = value
		}
		*m = ComposeStringMap(out)
		return nil
	default:
		return fmt.Errorf("line %d: value must be a mapping or KEY=VALUE list", node.Line)
	}
}

// ComposeListOrMap accepts Docker Compose options fields that can be either a
// string list or a mapping. Holos does not interpret these fields today, but
// retaining their shape avoids rejecting valid Compose files during strict
// decode.
type ComposeListOrMap struct {
	List []string
	Map  map[string]any
}

func (m *ComposeListOrMap) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			var value any
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[node.Content[i].Value] = value
		}
		m.Map = out
		m.List = nil
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			out = append(out, raw)
		}
		m.List = out
		m.Map = nil
		return nil
	default:
		return fmt.Errorf("line %d: value must be a mapping or list", node.Line)
	}
}

// IncludeFiles accepts Docker Compose include short syntax and long mapping
// syntax.
type IncludeFiles []IncludeFile

type IncludeFile struct {
	Path             StringList `yaml:"path,omitempty"`
	ProjectDirectory string     `yaml:"project_directory,omitempty"`
	EnvFile          StringList `yaml:"env_file,omitempty"`
}

func (i *IncludeFiles) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: include must be a list", node.Line)
	}
	out := make([]IncludeFile, 0, len(node.Content))
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, IncludeFile{Path: StringList{item.Value}})
		case yaml.MappingNode:
			var include IncludeFile
			if err := item.Decode(&include); err != nil {
				return err
			}
			out = append(out, include)
		default:
			return fmt.Errorf("line %d: include entries must be strings or mappings", item.Line)
		}
	}
	*i = IncludeFiles(out)
	return nil
}

// Network accepts Docker Compose top-level network declarations. Holos plans
// its VM network internally today, so these fields are retained as
// compatibility metadata.
type Network struct {
	Name       string         `yaml:"name,omitempty"`
	Driver     string         `yaml:"driver,omitempty"`
	DriverOpts map[string]any `yaml:"driver_opts,omitempty"`
	Attachable bool           `yaml:"attachable,omitempty"`
	Internal   bool           `yaml:"internal,omitempty"`
	External   any            `yaml:"external,omitempty"`
	Labels     ComposeLabels  `yaml:"labels,omitempty"`
	EnableIPv4 bool           `yaml:"enable_ipv4,omitempty"`
	EnableIPv6 bool           `yaml:"enable_ipv6,omitempty"`
	IPAM       IPAM           `yaml:"ipam,omitempty"`
}

type IPAM struct {
	Driver  string         `yaml:"driver,omitempty"`
	Config  []IPAMConfig   `yaml:"config,omitempty"`
	Options map[string]any `yaml:"options,omitempty"`
}

type IPAMConfig struct {
	Subnet       string            `yaml:"subnet,omitempty"`
	IPRange      string            `yaml:"ip_range,omitempty"`
	Gateway      string            `yaml:"gateway,omitempty"`
	AuxAddresses map[string]string `yaml:"aux_addresses,omitempty"`
}

// ServiceNetworks accepts Docker Compose service network list and mapping
// syntax.
type ServiceNetworks map[string]ServiceNetwork

type ServiceNetwork struct {
	Aliases      StringList     `yaml:"aliases,omitempty"`
	IPv4Address  string         `yaml:"ipv4_address,omitempty"`
	IPv6Address  string         `yaml:"ipv6_address,omitempty"`
	LinkLocalIPs StringList     `yaml:"link_local_ips,omitempty"`
	MacAddress   string         `yaml:"mac_address,omitempty"`
	Priority     any            `yaml:"priority,omitempty"`
	GWPriority   any            `yaml:"gw_priority,omitempty"`
	Interface    string         `yaml:"interface_name,omitempty"`
	DriverOpts   map[string]any `yaml:"driver_opts,omitempty"`
}

func (n *ServiceNetworks) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make(ServiceNetworks, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: network list entries must be scalar values", item.Line)
			}
			out[item.Value] = ServiceNetwork{}
		}
		*n = out
		return nil
	case yaml.MappingNode:
		out := make(ServiceNetworks, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			value := node.Content[i+1]
			if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
				out[name] = ServiceNetwork{}
				continue
			}
			var network ServiceNetwork
			if err := value.Decode(&network); err != nil {
				return err
			}
			out[name] = network
		}
		*n = out
		return nil
	default:
		return fmt.Errorf("line %d: networks must be a list or mapping", node.Line)
	}
}

// Config and Secret accept Docker Compose top-level resource declarations.
// Holos does not distribute these resources into guests yet; service references
// are retained so Compose files can be loaded strictly.
type Config struct {
	Name           string        `yaml:"name,omitempty"`
	File           string        `yaml:"file,omitempty"`
	Environment    string        `yaml:"environment,omitempty"`
	Content        string        `yaml:"content,omitempty"`
	TemplateDriver string        `yaml:"template_driver,omitempty"`
	External       any           `yaml:"external,omitempty"`
	Labels         ComposeLabels `yaml:"labels,omitempty"`
}

type Secret struct {
	Name           string         `yaml:"name,omitempty"`
	File           string         `yaml:"file,omitempty"`
	Environment    string         `yaml:"environment,omitempty"`
	External       any            `yaml:"external,omitempty"`
	Labels         ComposeLabels  `yaml:"labels,omitempty"`
	Driver         string         `yaml:"driver,omitempty"`
	DriverOpts     map[string]any `yaml:"driver_opts,omitempty"`
	TemplateDriver string         `yaml:"template_driver,omitempty"`
}

type ServiceResources []ServiceResource

type ServiceResource struct {
	Source string `yaml:"source,omitempty"`
	Target string `yaml:"target,omitempty"`
	UID    string `yaml:"uid,omitempty"`
	GID    string `yaml:"gid,omitempty"`
	Mode   any    `yaml:"mode,omitempty"`
}

func (r *ServiceResources) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: resource references must be a list", node.Line)
	}
	out := make([]ServiceResource, 0, len(node.Content))
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, ServiceResource{Source: item.Value})
		case yaml.MappingNode:
			var ref ServiceResource
			if err := item.Decode(&ref); err != nil {
				return err
			}
			out = append(out, ref)
		default:
			return fmt.Errorf("line %d: resource references must be strings or mappings", item.Line)
		}
	}
	*r = ServiceResources(out)
	return nil
}

type ComposeVolume struct {
	Short       string
	Type        string              `yaml:"type,omitempty"`
	Source      string              `yaml:"source,omitempty"`
	Target      string              `yaml:"target,omitempty"`
	ReadOnly    bool                `yaml:"read_only,omitempty"`
	Consistency string              `yaml:"consistency,omitempty"`
	Bind        ComposeVolumeBind   `yaml:"bind,omitempty"`
	Volume      ComposeVolumeVolume `yaml:"volume,omitempty"`
	Tmpfs       map[string]any      `yaml:"tmpfs,omitempty"`
	Image       map[string]any      `yaml:"image,omitempty"`
}

type ComposeVolumeBind struct {
	Propagation    string `yaml:"propagation,omitempty"`
	CreateHostPath *bool  `yaml:"create_host_path,omitempty"`
	SELinux        string `yaml:"selinux,omitempty"`
}

type ComposeVolumeVolume struct {
	NoCopy     bool              `yaml:"nocopy,omitempty"`
	Subpath    string            `yaml:"subpath,omitempty"`
	Labels     ComposeLabels     `yaml:"labels,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

func (v *ComposeVolume) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		v.Short = node.Value
		return nil
	case yaml.MappingNode:
		type rawVolume ComposeVolume
		return node.Decode((*rawVolume)(v))
	default:
		return fmt.Errorf("line %d: volume entries must be strings or mappings", node.Line)
	}
}

func (v ComposeVolume) MarshalYAML() (any, error) {
	if v.Short != "" {
		return v.Short, nil
	}
	type outVolume struct {
		Type        string              `yaml:"type,omitempty"`
		Source      string              `yaml:"source,omitempty"`
		Target      string              `yaml:"target,omitempty"`
		ReadOnly    bool                `yaml:"read_only,omitempty"`
		Consistency string              `yaml:"consistency,omitempty"`
		Bind        ComposeVolumeBind   `yaml:"bind,omitempty"`
		Volume      ComposeVolumeVolume `yaml:"volume,omitempty"`
		Tmpfs       map[string]any      `yaml:"tmpfs,omitempty"`
		Image       map[string]any      `yaml:"image,omitempty"`
	}
	return outVolume{
		Type:        v.Type,
		Source:      v.Source,
		Target:      v.Target,
		ReadOnly:    v.ReadOnly,
		Consistency: v.Consistency,
		Bind:        v.Bind,
		Volume:      v.Volume,
		Tmpfs:       v.Tmpfs,
		Image:       v.Image,
	}, nil
}

// EnvFiles accepts Docker Compose env_file string, list, and mapping entries.
type EnvFiles []EnvFile

type EnvFile struct {
	Path     string `yaml:"path"`
	Required any    `yaml:"required,omitempty"`
	Format   string `yaml:"format,omitempty"`
}

func (e *EnvFiles) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var path string
		if err := node.Decode(&path); err != nil {
			return err
		}
		*e = EnvFiles{{Path: path}}
		return nil
	case yaml.SequenceNode:
		out := make([]EnvFile, 0, len(node.Content))
		for _, item := range node.Content {
			entry, err := decodeEnvFile(item)
			if err != nil {
				return err
			}
			out = append(out, entry)
		}
		*e = EnvFiles(out)
		return nil
	default:
		return fmt.Errorf("line %d: env_file must be a string or list", node.Line)
	}
}

func decodeEnvFile(node *yaml.Node) (EnvFile, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		var path string
		if err := node.Decode(&path); err != nil {
			return EnvFile{}, err
		}
		return EnvFile{Path: path}, nil
	case yaml.MappingNode:
		allowed := map[string]struct{}{"path": {}, "required": {}, "format": {}}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if _, ok := allowed[key]; !ok {
				return EnvFile{}, fmt.Errorf("line %d: field %s not found in type compose.EnvFile", node.Content[i].Line, key)
			}
		}
		var entry EnvFile
		if err := node.Decode(&entry); err != nil {
			return EnvFile{}, err
		}
		return entry, nil
	default:
		return EnvFile{}, fmt.Errorf("line %d: env_file entries must be strings or mappings", node.Line)
	}
}

func (e EnvFile) required() bool {
	switch required := e.Required.(type) {
	case nil:
		return true
	case bool:
		return required
	case string:
		parsed, err := strconv.ParseBool(required)
		if err == nil {
			return parsed
		}
		return true
	default:
		return true
	}
}

// Environment accepts Docker Compose environment map syntax and list syntax.
// Nil values represent unset variables and are omitted from rendered guest
// environment files.
type Environment map[string]*string

func (e *Environment) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]*string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]
			if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
				out[key] = nil
				continue
			}
			var raw string
			if err := value.Decode(&raw); err != nil {
				return err
			}
			out[key] = stringPtr(raw)
		}
		*e = Environment(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]*string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			key, value, ok := strings.Cut(raw, "=")
			if !ok {
				out[key] = nil
				continue
			}
			out[key] = stringPtr(value)
		}
		*e = Environment(out)
		return nil
	default:
		return fmt.Errorf("line %d: environment must be a mapping or list", node.Line)
	}
}

func stringPtr(s string) *string {
	return &s
}

// ExtraHosts accepts Docker Compose extra_hosts map syntax and list syntax.
// The list form accepts HOST=IP and HOST:IP, matching Compose.
type ExtraHosts map[string]string

func (e *ExtraHosts) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[node.Content[i].Value] = trimBracketedIPv6(value)
		}
		*e = ExtraHosts(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			host, addr, err := parseExtraHost(raw)
			if err != nil {
				return err
			}
			out[host] = addr
		}
		*e = ExtraHosts(out)
		return nil
	default:
		return fmt.Errorf("line %d: extra_hosts must be a mapping or list", node.Line)
	}
}

func parseExtraHost(raw string) (string, string, error) {
	host, addr, ok := strings.Cut(raw, "=")
	if !ok {
		host, addr, ok = strings.Cut(raw, ":")
	}
	if !ok || host == "" || addr == "" {
		return "", "", fmt.Errorf("invalid extra_hosts entry %q: expected host=ip or host:ip", raw)
	}
	return host, trimBracketedIPv6(addr), nil
}

func trimBracketedIPv6(addr string) string {
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
	}
	return addr
}

// ComposeLabels accepts Docker Compose's label map syntax and list syntax.
// A list entry without "=" is a label with an empty value.
type ComposeLabels map[string]string

func (l *ComposeLabels) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[node.Content[i].Value] = value
		}
		*l = ComposeLabels(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			key, value, ok := strings.Cut(raw, "=")
			if !ok {
				value = ""
			}
			out[key] = value
		}
		*l = ComposeLabels(out)
		return nil
	default:
		return fmt.Errorf("line %d: labels must be a mapping or list", node.Line)
	}
}

// DependsOn accepts Docker Compose's short list syntax and long mapping
// syntax. Holos resolves both to a deterministic service-name list used for
// topological ordering; long-form options are accepted for Compose-file
// compatibility.
type DependsOn []string

func (d *DependsOn) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var names []string
		if err := node.Decode(&names); err != nil {
			return err
		}
		*d = DependsOn(names)
		return nil
	case yaml.MappingNode:
		names := make([]string, 0, len(node.Content)/2)
		allowed := map[string]struct{}{
			"condition": {},
			"restart":   {},
			"required":  {},
		}
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			value := node.Content[i+1]
			if value.Kind == yaml.MappingNode {
				for j := 0; j < len(value.Content); j += 2 {
					key := value.Content[j].Value
					if _, ok := allowed[key]; !ok {
						return fmt.Errorf("line %d: field %s not found in type compose.DependsOn", value.Content[j].Line, key)
					}
				}
				var opts struct {
					Condition string `yaml:"condition"`
					Restart   *bool  `yaml:"restart"`
					Required  *bool  `yaml:"required"`
				}
				if err := value.Decode(&opts); err != nil {
					return err
				}
				switch opts.Condition {
				case "", "service_started", "service_healthy", "service_completed_successfully":
				default:
					return fmt.Errorf("line %d: unsupported depends_on condition %q", value.Line, opts.Condition)
				}
			} else if value.Kind != yaml.ScalarNode || value.Tag != "!!null" {
				return fmt.Errorf("line %d: depends_on service %q must be a mapping", value.Line, name)
			}
			names = append(names, name)
		}
		sort.Strings(names)
		*d = DependsOn(names)
		return nil
	default:
		return fmt.Errorf("line %d: depends_on must be a list or mapping", node.Line)
	}
}

func (d DependsOn) MarshalYAML() (any, error) {
	return []string(d), nil
}

// ComposePort accepts Docker Compose's short scalar port syntax
// ("127.0.0.1:8080:80/tcp") and long mapping syntax
// ({target, published, host_ip, protocol, ...}). Both forms resolve to
// config.PortForward in parseComposePort.
type ComposePort struct {
	Short       string
	Name        string
	Target      int
	Published   string
	HostIP      string
	Protocol    string
	AppProtocol string
	Mode        string
	hasTarget   bool
}

func (p *ComposePort) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		p.Short = node.Value
		return nil
	case yaml.MappingNode:
		allowed := map[string]struct{}{
			"name":         {},
			"target":       {},
			"published":    {},
			"host_ip":      {},
			"protocol":     {},
			"app_protocol": {},
			"mode":         {},
		}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if _, ok := allowed[key]; !ok {
				return fmt.Errorf("line %d: field %s not found in type compose.ComposePort", node.Content[i].Line, key)
			}
			value := node.Content[i+1]
			switch key {
			case "name":
				if err := value.Decode(&p.Name); err != nil {
					return err
				}
			case "target":
				var target string
				if err := decodeComposeStringOrInt(value, &target); err != nil {
					return err
				}
				parsed, err := strconv.Atoi(target)
				if err != nil {
					return fmt.Errorf("target: %w", err)
				}
				p.Target = parsed
				p.hasTarget = true
			case "published":
				if err := decodeComposeStringOrInt(value, &p.Published); err != nil {
					return fmt.Errorf("published: %w", err)
				}
			case "host_ip":
				if err := value.Decode(&p.HostIP); err != nil {
					return err
				}
			case "protocol":
				if err := value.Decode(&p.Protocol); err != nil {
					return err
				}
			case "app_protocol":
				if err := value.Decode(&p.AppProtocol); err != nil {
					return err
				}
			case "mode":
				if err := value.Decode(&p.Mode); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("line %d: ports entries must be a string or mapping", node.Line)
	}
}

func (p ComposePort) MarshalYAML() (any, error) {
	if p.Short != "" {
		return p.Short, nil
	}
	return struct {
		Name        string `yaml:"name,omitempty"`
		Target      int    `yaml:"target"`
		Published   string `yaml:"published,omitempty"`
		HostIP      string `yaml:"host_ip,omitempty"`
		Protocol    string `yaml:"protocol,omitempty"`
		AppProtocol string `yaml:"app_protocol,omitempty"`
		Mode        string `yaml:"mode,omitempty"`
	}{
		Name:        p.Name,
		Target:      p.Target,
		Published:   p.Published,
		HostIP:      p.HostIP,
		Protocol:    p.Protocol,
		AppProtocol: p.AppProtocol,
		Mode:        p.Mode,
	}, nil
}

func decodeComposeStringOrInt(node *yaml.Node, out *string) error {
	var s string
	if err := node.Decode(&s); err == nil {
		*out = s
		return nil
	}
	var i int
	if err := node.Decode(&i); err == nil {
		*out = strconv.Itoa(i)
		return nil
	}
	return fmt.Errorf("must be a string or integer")
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
	// StartInterval is accepted for Docker Compose compatibility. Holos uses
	// Interval for probing today.
	StartInterval string `yaml:"start_interval,omitempty"`
	// Timeout bounds a single probe's wall-clock budget. Defaults
	// to 5s.
	Timeout string `yaml:"timeout,omitempty"`
	// Disable accepts Docker Compose's explicit healthcheck opt-out.
	Disable bool `yaml:"disable,omitempty"`
}

// UnmarshalYAML accepts Healthcheck.Test as either a list of strings
// (canonical docker-compose form) or a single shell string. The single-
// string form is wrapped in ["sh", "-c", ...] so it runs through the
// shell exactly like docker-compose's CMD-SHELL variant.
//
// The outer Load() uses yaml.Decoder.KnownFields(true), but that
// setting is lost as soon as a custom UnmarshalYAML takes over:
// yaml.Node.Decode has no equivalent toggle. To keep the same
// strict-typo guarantee ("retriez:" is an error, not a silently
// dropped field), we explicitly enumerate this struct's keys and
// reject anything else before calling node.Decode.
func (h *Healthcheck) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		allowed := map[string]struct{}{
			"test":           {},
			"interval":       {},
			"retries":        {},
			"start_period":   {},
			"start_interval": {},
			"timeout":        {},
			"disable":        {},
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if _, ok := allowed[key]; !ok {
				return fmt.Errorf("line %d: field %s not found in type compose.Healthcheck", node.Content[i].Line, key)
			}
		}
	}

	type rawHealthcheck struct {
		Test          yaml.Node `yaml:"test"`
		Interval      string    `yaml:"interval"`
		Retries       int       `yaml:"retries"`
		StartPeriod   string    `yaml:"start_period"`
		StartInterval string    `yaml:"start_interval"`
		Timeout       string    `yaml:"timeout"`
		Disable       bool      `yaml:"disable"`
	}
	var raw rawHealthcheck
	if err := node.Decode(&raw); err != nil {
		return err
	}

	h.Interval = raw.Interval
	h.Retries = raw.Retries
	h.StartPeriod = raw.StartPeriod
	h.StartInterval = raw.StartInterval
	h.Timeout = raw.Timeout
	h.Disable = raw.Disable

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

// ComposeDevice is a PCI device to pass through to the VM via VFIO.
type ComposeDevice struct {
	Raw         string
	PCI         string `yaml:"pci,omitempty"`
	ROMFile     string `yaml:"rom_file,omitempty"`
	Source      string `yaml:"source,omitempty"`
	Target      string `yaml:"target,omitempty"`
	Permissions string `yaml:"permissions,omitempty"`
}

func (d *ComposeDevice) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		d.Raw = node.Value
		return nil
	case yaml.MappingNode:
		type rawDevice ComposeDevice
		return node.Decode((*rawDevice)(d))
	default:
		return fmt.Errorf("line %d: devices entries must be strings or mappings", node.Line)
	}
}

func (d ComposeDevice) MarshalYAML() (any, error) {
	if d.Raw != "" {
		return d.Raw, nil
	}
	type outDevice struct {
		PCI         string `yaml:"pci,omitempty"`
		ROMFile     string `yaml:"rom_file,omitempty"`
		Source      string `yaml:"source,omitempty"`
		Target      string `yaml:"target,omitempty"`
		Permissions string `yaml:"permissions,omitempty"`
	}
	return outDevice{
		PCI:         d.PCI,
		ROMFile:     d.ROMFile,
		Source:      d.Source,
		Target:      d.Target,
		Permissions: d.Permissions,
	}, nil
}

// CloudInit holds cloud-init configuration embedded in the compose file.
type CloudInit struct {
	Hostname          string      `yaml:"hostname,omitempty"`
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
//
// Decoding is strict (KnownFields(true)) so typos like `portz:` or
// `volume:` (singular) fail loudly instead of being silently dropped.
// docker-compose users hit this regularly with `enviroment:` and
// `volums:`; the YAML round-trips fine and the misspelled key just
// vanishes, leaving them debugging missing port mappings or volume
// mounts. We'd rather refuse to load.
func Load(path string) (*File, error) {
	return load(path, map[string]bool{})
}

func load(path string, seen map[string]bool) (*File, error) {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if seen[path] {
		return nil, fmt.Errorf("include cycle involving %s", path)
	}
	seen[path] = true
	defer delete(seen, path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}

	data, err = stripExtensionFields(data)
	if err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var file File
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	if file.Name == "" {
		abs, err := filepath.Abs(path)
		if err == nil {
			file.Name = filepath.Base(filepath.Dir(abs))
		}
	}
	file.baseDir = filepath.Dir(path)

	if err := mergeIncludes(&file, filepath.Dir(path), seen); err != nil {
		return nil, err
	}
	return &file, nil
}

func mergeIncludes(file *File, baseDir string, seen map[string]bool) error {
	for _, include := range file.Include {
		projectDir := baseDir
		if include.ProjectDirectory != "" {
			projectDir = include.ProjectDirectory
			if !filepath.IsAbs(projectDir) {
				projectDir = filepath.Join(baseDir, projectDir)
			}
		}
		for _, includePath := range include.Path {
			path := includePath
			if !filepath.IsAbs(path) {
				path = filepath.Join(projectDir, path)
			}
			if _, err := os.Stat(path); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return fmt.Errorf("include %q: %w", includePath, err)
			}
			included, err := load(path, seen)
			if err != nil {
				return fmt.Errorf("include %q: %w", includePath, err)
			}
			if included.serviceBaseDirs == nil {
				included.serviceBaseDirs = map[string]string{}
			}
			for name := range included.Services {
				if _, exists := included.serviceBaseDirs[name]; !exists {
					included.serviceBaseDirs[name] = projectDir
				}
			}
			mergeIncludedFile(file, included)
		}
	}
	return nil
}

func mergeIncludedFile(dst *File, src *File) {
	mergeMap(&dst.Services, src.Services)
	mergeMap(&dst.Volumes, src.Volumes)
	mergeMap(&dst.Networks, src.Networks)
	mergeMap(&dst.Configs, src.Configs)
	mergeMap(&dst.Secrets, src.Secrets)
	mergeMap(&dst.Models, src.Models)
	if dst.serviceBaseDirs == nil {
		dst.serviceBaseDirs = map[string]string{}
	}
	for name, dir := range src.serviceBaseDirs {
		if _, exists := dst.serviceBaseDirs[name]; !exists {
			dst.serviceBaseDirs[name] = dir
		}
	}
}

func mergeMap[M ~map[string]V, V any](dst *M, src M) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = M{}
	}
	for key, value := range src {
		if _, exists := (*dst)[key]; !exists {
			(*dst)[key] = value
		}
	}
}

func stripExtensionFields(data []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	normalizeComposeYAMLNode(&doc)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func normalizeComposeYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Tag {
	case "!override":
		node.Tag = ""
	case "!reset":
		resetYAMLNode(node)
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			normalizeComposeYAMLNode(child)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			normalizeComposeYAMLNode(child)
		}
	case yaml.MappingNode:
		out := node.Content[:0]
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind == yaml.ScalarNode && strings.HasPrefix(key.Value, "x-") {
				continue
			}
			normalizeComposeYAMLNode(value)
			out = append(out, key, value)
		}
		node.Content = out
	case yaml.AliasNode:
		if node.Alias != nil {
			*node = cloneYAMLNode(node.Alias)
			normalizeComposeYAMLNode(node)
		}
	}
}

func cloneYAMLNode(node *yaml.Node) yaml.Node {
	if node == nil {
		return yaml.Node{}
	}
	clone := *node
	if len(node.Content) > 0 {
		clone.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			childClone := cloneYAMLNode(child)
			clone.Content[i] = &childClone
		}
	}
	if node.Alias != nil {
		aliasClone := cloneYAMLNode(node.Alias)
		clone.Alias = &aliasClone
	}
	return clone
}

func resetYAMLNode(node *yaml.Node) {
	switch node.Kind {
	case yaml.SequenceNode:
		node.Tag = "!!seq"
		node.Content = nil
	case yaml.MappingNode:
		node.Tag = "!!map"
		node.Content = nil
	default:
		node.Kind = yaml.ScalarNode
		node.Tag = "!!null"
		node.Value = ""
		node.Content = nil
	}
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

	// Replicas must be validated before the IP allocation below: a
	// negative or absurdly large value would otherwise reach
	// `make([]string, replicas)` and panic before manifest validation
	// ever ran. We accept zero (treated as the documented default of
	// 1) and reject anything else outside [1, maxReplicas]. The
	// upper bound is a soft guard against typos like `replicas:
	// 1000000` that would happily allocate a million addresses.
	//
	// Project-wide, the internal network is 10.10.0.0/24 with the
	// pool starting at .2 (.1 is the gateway/host placeholder),
	// leaving .2-.254 or 253 usable addresses. A project that asks
	// for more instances than the subnet can hold is rejected up
	// front so the allocator never emits nonsense like 10.10.0.270;
	// previously two services with replicas: 200 + 100 passed
	// validation and produced invalid IPs the runtime would then
	// fail to route.
	totalReplicas := 0
	for _, name := range order {
		svc := f.Services[name]
		replicas, err := serviceReplicas(svc)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		totalReplicas += replicas
	}
	if totalReplicas > maxProjectInstances {
		return nil, fmt.Errorf(
			"project requires %d instances but the internal network %s only has %d usable addresses",
			totalReplicas, subnetCIDR, maxProjectInstances)
	}

	for _, name := range order {
		svc := f.Services[name]
		replicas, _ := serviceReplicas(svc)

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
		serviceBaseDir := baseDir
		if f.serviceBaseDirs != nil && f.serviceBaseDirs[name] != "" {
			serviceBaseDir = f.serviceBaseDirs[name]
		}
		manifest, err := f.resolveService(name, svc, serviceBaseDir, cacheDir, network, hosts, serviceIPs[name])
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		// Run the canonical Manifest validator on every resolved
		// service so out-of-range fields (memory_mb, port numbers,
		// healthcheck timing, ...) are caught at compose load time
		// instead of surfacing as a runtime panic or, worse, a
		// silently misconfigured VM. Without this, `holos validate`
		// can return success on YAML the runtime will reject.
		if err := manifest.Validate(); err != nil {
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

func composeInt(value any, label string) (int, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("%s must be an integer", label)
		}
		return int(v), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", label, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", label, value)
	}
}

func composeFloat(value any, label string) (float64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", label, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s has unsupported type %T", label, value)
	}
}

func serviceReplicas(svc Service) (int, error) {
	scale, err := composeInt(svc.Scale, "scale")
	if err != nil {
		return 0, err
	}
	if svc.Replicas != 0 && scale != 0 && svc.Replicas != scale {
		return 0, fmt.Errorf("replicas and scale disagree (%d != %d)", svc.Replicas, scale)
	}
	if svc.Replicas != 0 && svc.Deploy.Replicas != 0 && svc.Replicas != svc.Deploy.Replicas {
		return 0, fmt.Errorf("replicas and deploy.replicas disagree (%d != %d)", svc.Replicas, svc.Deploy.Replicas)
	}
	if scale != 0 && svc.Deploy.Replicas != 0 && scale != svc.Deploy.Replicas {
		return 0, fmt.Errorf("scale and deploy.replicas disagree (%d != %d)", scale, svc.Deploy.Replicas)
	}
	replicas := svc.Replicas
	if replicas == 0 {
		replicas = scale
	}
	if replicas == 0 {
		replicas = svc.Deploy.Replicas
	}
	if replicas == 0 {
		replicas = config.DefaultReplicas
	}
	if replicas < 1 {
		return 0, fmt.Errorf("replicas must be >= 1")
	}
	if replicas > maxReplicas {
		return 0, fmt.Errorf("replicas %d exceeds maximum of %d", replicas, maxReplicas)
	}
	return replicas, nil
}

func composeCPUs(svc Service) (float64, error) {
	cpus, err := composeFloat(svc.CPUs, "cpus")
	if err != nil {
		return 0, err
	}
	if cpus > 0 {
		return cpus, nil
	}
	for _, candidate := range []struct {
		label string
		raw   string
	}{
		{"deploy.resources.limits.cpus", svc.Deploy.Resources.Limits.CPUs},
		{"deploy.resources.reservations.cpus", svc.Deploy.Resources.Reservations.CPUs},
	} {
		raw := candidate.raw
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", candidate.label, err)
		}
		return value, nil
	}
	return 0, nil
}

func composeVCPU(cpus float64) int {
	if cpus <= 0 {
		return config.DefaultVCPU
	}
	return int(math.Ceil(cpus))
}

func composeMemLimit(svc Service) string {
	if strings.TrimSpace(svc.MemLimit) != "" {
		return svc.MemLimit
	}
	if strings.TrimSpace(svc.Deploy.Resources.Limits.Memory) != "" {
		return svc.Deploy.Resources.Limits.Memory
	}
	return svc.Deploy.Resources.Reservations.Memory
}

func composeMemoryMB(memLimit string) (int, error) {
	if strings.TrimSpace(memLimit) == "" {
		return config.DefaultMemoryMB, nil
	}
	bytes, err := parseVolumeSize(memLimit)
	if err != nil {
		return 0, fmt.Errorf("mem_limit: %w", err)
	}
	mb := int(bytes / (1 << 20))
	if bytes%(1<<20) != 0 {
		mb++
	}
	return mb, nil
}

func (f *File) resolveService(name string, svc Service, baseDir string, cacheDir string, network NetworkPlan, hosts map[string]string, instanceIPs []string) (config.Manifest, error) {
	replicas, err := serviceReplicas(svc)
	if err != nil {
		return config.Manifest{}, err
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
	if svc.Dockerfile != "" || svc.Build.isSet() {
		dfPath := svc.Dockerfile
		dfContext := ""
		if !filepath.IsAbs(dfPath) {
			dfPath = filepath.Join(baseDir, dfPath)
		}
		if svc.Dockerfile == "" {
			var ok bool
			dfPath, dfContext, ok, err = svc.Build.dockerfilePath(baseDir)
			if err != nil {
				return config.Manifest{}, err
			}
			if !ok {
				return config.Manifest{}, fmt.Errorf("build: dockerfile path is required")
			}
		}
		if dfContext == "" {
			dfContext = filepath.Dir(dfPath)
		}
		var dfResult *dockerfile.Result
		if svc.Dockerfile == "" && strings.TrimSpace(svc.Build.DockerfileInline) != "" {
			dfResult, err = dockerfile.ParseContent(svc.Build.DockerfileInline, dfContext)
		} else {
			dfResult, err = dockerfile.Parse(dfPath, dfContext)
		}
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
	imageOS := svc.ImageOS
	if imageOS == "" {
		imageOS = images.OSFamily(svc.Image)
	}
	if imageOS == "" {
		imageOS = config.ImageOSSystemd
	}

	vcpu := svc.VM.VCPU
	if vcpu == 0 {
		cpus, err := composeCPUs(svc)
		if err != nil {
			return config.Manifest{}, err
		}
		vcpu = composeVCPU(cpus)
	}
	memMB := svc.VM.MemoryMB
	if memMB == 0 {
		memMB, err = composeMemoryMB(composeMemLimit(svc))
		if err != nil {
			return config.Manifest{}, err
		}
	}
	var diskSizeBytes int64
	if strings.TrimSpace(svc.VM.DiskSize) != "" {
		diskSizeBytes, err = parseVolumeSize(svc.VM.DiskSize)
		if err != nil {
			return config.Manifest{}, fmt.Errorf("vm.disk_size: %w", err)
		}
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
	//   2. docker-compose-compatible service.user
	//   3. the image's conventional cloud user (debian → "debian",
	//      alpine → "alpine", etc.) so cloud-init creates an account
	//      that matches what the rest of the ecosystem expects
	//   4. the global default ("ubuntu")
	// Using the image default in the middle slot is what keeps
	// `holos run debian` from producing a VM whose console autologin
	// fails because no `ubuntu` user materialised.
	user := svc.CloudInit.User
	if user == "" {
		user = svc.User
	}
	if user == "" {
		user = images.DefaultUser(svc.Image)
	}
	if user == "" {
		user = config.DefaultUser
	}
	if err := config.ValidateUserName(user); err != nil {
		return config.Manifest{}, fmt.Errorf("cloud_init.user: %w", err)
	}
	hostname := svc.CloudInit.Hostname
	if hostname == "" {
		hostname = composeHostname(svc.Hostname, svc.DomainName)
	}

	writeFiles := make([]config.WriteFile, 0, len(dfWriteFiles)+len(svc.CloudInit.WriteFiles))
	writeFiles = append(writeFiles, dfWriteFiles...)
	env, err := resolveEnvironment(baseDir, svc.EnvFile, svc.Environment)
	if err != nil {
		return config.Manifest{}, err
	}
	if envFile, ok := environmentFile(env); ok {
		writeFiles = append(writeFiles, envFile)
	}
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

	devices := make([]config.Device, 0, len(svc.Devices))
	for i, d := range svc.Devices {
		if d.Raw != "" || d.PCI == "" {
			continue
		}
		pci := normalizePCIAddress(d.PCI)
		if err := config.ValidatePCIAddress(pci); err != nil {
			return config.Manifest{}, fmt.Errorf("device %d pci %q: %w", i, d.PCI, err)
		}
		devices = append(devices, config.Device{
			PCI:     pci,
			ROMFile: d.ROMFile,
		})
	}

	uefi := svc.VM.UEFI
	if !uefi && len(devices) > 0 {
		uefi = true
	}
	extraArgs := append([]string(nil), svc.VM.ExtraArgs...)
	if !uefi && images.RequiresVGA(svc.Image) {
		// Debian 13's BIOS GRUB path can stall at "Booting Debian GNU/Linux"
		// with holos' -nodefaults serial-only device layout. Supplying a
		// headless VGA device satisfies GRUB's gfxterm setup while keeping the
		// serial console and QEMU display disabled.
		extraArgs = append([]string{"-device", "VGA"}, extraArgs...)
	}

	gracePeriodSec, err := parseStopGracePeriod(svc.StopGracePeriod)
	if err != nil {
		return config.Manifest{}, err
	}

	healthcheck, err := resolveHealthcheck(svc.Healthcheck)
	if err != nil {
		return config.Manifest{}, err
	}

	extraHosts := make(map[string]string, len(hosts)+len(svc.ExtraHosts))
	for host, addr := range hosts {
		extraHosts[host] = addr
	}
	for host, addr := range svc.ExtraHosts {
		extraHosts[host] = addr
	}

	labels, err := resolveLabels(baseDir, svc.LabelFile, svc.Labels)
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
		ImageOS:     imageOS,
		VM: config.VMConfig{
			VCPU:          vcpu,
			MemoryMB:      memMB,
			DiskSizeBytes: diskSizeBytes,
			Machine:       machine,
			CPUModel:      cpuModel,
			UEFI:          uefi,
			ExtraArgs:     extraArgs,
		},
		Devices: devices,
		Labels:  labels,
		Network: config.NetworkConfig{Mode: "user"},
		Ports:   ports,
		Mounts:  mounts,
		CloudInit: config.CloudInit{
			Hostname:          hostname,
			User:              user,
			SSHAuthorizedKeys: svc.CloudInit.SSHAuthorizedKeys,
			Packages:          svc.CloudInit.Packages,
			WriteFiles:        writeFiles,
			RunCmd:            serviceRunCmd(svc, dfRunCmd),
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
		ExtraHosts:         extraHosts,
		StopGracePeriodSec: gracePeriodSec,
		Healthcheck:        healthcheck,
		DependsOn:          append([]string(nil), svc.DependsOn...),
	}, nil
}

func environmentFile(env Environment) (config.WriteFile, bool) {
	if len(env) == 0 {
		return config.WriteFile{}, false
	}
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if value != nil {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return config.WriteFile{}, false
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%q\n", key, *env[key])
	}
	return config.WriteFile{
		Path:        "/etc/environment",
		Content:     b.String(),
		Permissions: "0644",
		Owner:       "root:root",
	}, true
}

func composeRunCmd(entrypoint, command ComposeCommand, workingDir string) []string {
	parts := make([]string, 0, len(entrypoint.Args)+len(command.Args))
	if len(entrypoint.Args) > 0 {
		parts = append(parts, entrypoint.shellFragment())
	}
	if len(command.Args) > 0 {
		parts = append(parts, command.shellFragment())
	}
	if len(parts) == 0 {
		return nil
	}
	cmd := strings.Join(parts, " ")
	if workingDir != "" {
		cmd = "cd " + shellQuote(workingDir) + " && " + cmd
	}
	return []string{cmd}
}

func serviceRunCmd(svc Service, dfRunCmd []string) []string {
	out := append([]string{}, dfRunCmd...)
	out = append(out, composeRunCmd(svc.Entrypoint, svc.Command, svc.WorkingDir)...)
	out = append(out, lifecycleRunCmd(svc.PostStart)...)
	out = append(out, lifecycleRunCmd(svc.PreStop)...)
	out = append(out, svc.CloudInit.RunCmd...)
	return out
}

func lifecycleRunCmd(hooks []LifecycleHook) []string {
	var out []string
	for _, hook := range hooks {
		cmds := composeRunCmd(ComposeCommand{}, hook.Command, hook.WorkingDir)
		for _, cmd := range cmds {
			if env := environmentPrefix(hook.Environment); env != "" {
				cmd = env + " " + cmd
			}
			out = append(out, cmd)
		}
	}
	return out
}

func environmentPrefix(env Environment) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for key, value := range env {
		if value != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+shellQuote(*env[key]))
	}
	return strings.Join(parts, " ")
}

func (c ComposeCommand) shellFragment() string {
	if c.Scalar {
		return c.Args[0]
	}
	return shellJoin(c.Args)
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') &&
			!(r >= 'a' && r <= 'z') &&
			!(r >= '0' && r <= '9') &&
			!strings.ContainsRune("_+-=/:.,@", r)
	}) == -1 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

func resolveEnvironment(baseDir string, files EnvFiles, inline Environment) (Environment, error) {
	out := Environment{}
	for _, file := range files {
		if file.Path == "" {
			return nil, fmt.Errorf("env_file path is required")
		}
		if file.Format != "" && file.Format != "raw" {
			return nil, fmt.Errorf("env_file format %q is unsupported; only raw is implemented", file.Format)
		}
		path := file.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		env, err := readEnvFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && !file.required() {
				continue
			}
			return nil, fmt.Errorf("env_file %q: %w", file.Path, err)
		}
		for key, value := range env {
			out[key] = value
		}
	}
	for key, value := range inline {
		out[key] = value
	}
	return out, nil
}

func resolveLabels(baseDir string, files StringList, inline ComposeLabels) (map[string]string, error) {
	out := map[string]string{}
	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		labels, err := readEnvFile(path)
		if err != nil {
			return nil, fmt.Errorf("label_file %q: %w", file, err)
		}
		for key, value := range labels {
			if value == nil {
				out[key] = ""
			} else {
				out[key] = *value
			}
		}
	}
	for key, value := range inline {
		out[key] = value
	}
	return out, nil
}

func readEnvFile(path string) (Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := Environment{}
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			out[line] = nil
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty variable name", lineNo+1)
		}
		out[key] = stringPtr(strings.TrimSpace(value))
	}
	return out, nil
}

// resolveHealthcheck validates and normalises a compose healthcheck
// block into the resolved config form. Absent blocks pass through as
// nil so consumers never have to check zero-value fields.
func resolveHealthcheck(h *Healthcheck) (*config.HealthcheckConfig, error) {
	if h == nil {
		return nil, nil
	}
	if h.Disable || (len(h.Test) == 1 && h.Test[0] == "NONE") {
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

func composeHostname(hostname, domainName string) string {
	if hostname == "" {
		return ""
	}
	if domainName == "" || strings.Contains(hostname, ".") {
		return hostname
	}
	return hostname + "." + domainName
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
		if svc.Image == "" && svc.Dockerfile == "" && !svc.Build.isSet() {
			return fmt.Errorf("service %q requires an image (or a dockerfile/build with FROM)", name)
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

func parsePorts(specs []ComposePort) ([]config.PortForward, error) {
	ports := make([]config.PortForward, 0, len(specs))
	for i, spec := range specs {
		parsed, err := parseComposePort(spec)
		if err != nil {
			return nil, fmt.Errorf("port %d: %w", i, err)
		}
		for j := range parsed {
			if parsed[j].Name == "" {
				if len(parsed) == 1 {
					parsed[j].Name = fmt.Sprintf("port-%d", i)
				} else {
					parsed[j].Name = fmt.Sprintf("port-%d-%d", i, j)
				}
			}
			ports = append(ports, parsed[j])
		}
	}
	return ports, nil
}

func parseComposePort(port ComposePort) ([]config.PortForward, error) {
	if port.Short != "" {
		return parsePort(port.Short)
	}
	if !port.hasTarget && port.Target == 0 {
		return nil, fmt.Errorf("target is required")
	}
	protocol := port.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	if protocol != "tcp" {
		return nil, fmt.Errorf("protocol %q is unsupported; only tcp is implemented", protocol)
	}
	if port.Mode != "" && port.Mode != "host" && port.Mode != "ingress" {
		return nil, fmt.Errorf("mode %q is unsupported; expected host or ingress", port.Mode)
	}

	var hostAddr string
	if port.HostIP != "" {
		addr, err := parsePortAddress("host", port.HostIP)
		if err != nil {
			return nil, err
		}
		hostAddr = addr
	}

	hostPorts := []int{0}
	if port.Published != "" {
		parsed, err := parsePortRange(port.Published)
		if err != nil {
			return nil, fmt.Errorf("invalid published port: %w", err)
		}
		hostPorts = parsed
	}

	out := make([]config.PortForward, 0, len(hostPorts))
	for _, hostPort := range hostPorts {
		out = append(out, config.PortForward{
			Name:      port.Name,
			HostAddr:  hostAddr,
			HostPort:  hostPort,
			GuestPort: port.Target,
			Protocol:  protocol,
		})
	}
	return out, nil
}

func parsePort(spec string) ([]config.PortForward, error) {
	protocol := "tcp"
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}
	// Only TCP forwarding is implemented end-to-end; reject other
	// protocols at parse time rather than let the user discover the
	// limitation at `holos up` via a validation error.
	if protocol != "tcp" {
		return nil, fmt.Errorf("protocol %q is unsupported; only tcp is implemented", protocol)
	}

	parts, err := splitPortSpec(spec)
	if err != nil {
		return nil, err
	}
	switch len(parts) {
	case 1:
		guests, err := parsePortRange(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		out := make([]config.PortForward, 0, len(guests))
		for _, guest := range guests {
			out = append(out, config.PortForward{GuestPort: guest, Protocol: protocol})
		}
		return out, nil
	case 2:
		hosts, err := parsePortRange(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid host port: %w", err)
		}
		guests, err := parsePortRange(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid guest port: %w", err)
		}
		return expandPortRanges(hosts, guests, "", "", protocol)
	case 3:
		hostAddr, err := parsePortAddress("host", parts[0])
		if err != nil {
			return nil, err
		}
		hosts, err := parsePortRange(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid host port: %w", err)
		}
		guests, err := parsePortRange(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid guest port: %w", err)
		}
		return expandPortRanges(hosts, guests, hostAddr, "", protocol)
	case 4:
		hostAddr, err := parsePortAddress("host", parts[0])
		if err != nil {
			return nil, err
		}
		hosts, err := parsePortRange(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid host port: %w", err)
		}
		guestAddr, err := parsePortAddress("guest", parts[2])
		if err != nil {
			return nil, err
		}
		guests, err := parsePortRange(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid guest port: %w", err)
		}
		return expandPortRanges(hosts, guests, hostAddr, guestAddr, protocol)
	default:
		return nil, fmt.Errorf("invalid port spec")
	}
}

func splitPortSpec(spec string) ([]string, error) {
	var parts []string
	var b strings.Builder
	inBrackets := false
	for _, r := range spec {
		switch r {
		case '[':
			if inBrackets {
				return nil, fmt.Errorf("invalid port spec")
			}
			inBrackets = true
			b.WriteRune(r)
		case ']':
			if !inBrackets {
				return nil, fmt.Errorf("invalid port spec")
			}
			inBrackets = false
			b.WriteRune(r)
		case ':':
			if inBrackets {
				b.WriteRune(r)
				continue
			}
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if inBrackets {
		return nil, fmt.Errorf("invalid port spec")
	}
	parts = append(parts, b.String())
	return parts, nil
}

func parsePortRange(raw string) ([]int, error) {
	startRaw, endRaw, hasRange := strings.Cut(raw, "-")
	start, err := strconv.Atoi(startRaw)
	if err != nil {
		return nil, err
	}
	if !hasRange {
		return []int{start}, nil
	}
	end, err := strconv.Atoi(endRaw)
	if err != nil {
		return nil, err
	}
	if end < start {
		return nil, fmt.Errorf("range end must be >= start")
	}
	out := make([]int, 0, end-start+1)
	for port := start; port <= end; port++ {
		out = append(out, port)
	}
	return out, nil
}

func expandPortRanges(hosts, guests []int, hostAddr, guestAddr, protocol string) ([]config.PortForward, error) {
	if len(hosts) != len(guests) {
		return nil, fmt.Errorf("host and guest port ranges must have the same length")
	}
	out := make([]config.PortForward, 0, len(hosts))
	for i := range hosts {
		out = append(out, config.PortForward{
			HostAddr:  hostAddr,
			HostPort:  hosts[i],
			GuestAddr: guestAddr,
			GuestPort: guests[i],
			Protocol:  protocol,
		})
	}
	return out, nil
}

func parsePortAddress(kind, raw string) (string, error) {
	raw = strings.TrimPrefix(strings.TrimSuffix(raw, "]"), "[")
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s address: %w", kind, err)
	}
	if !addr.Is4() {
		return "", fmt.Errorf("invalid %s address %q: only IPv4 addresses are supported", kind, raw)
	}
	return addr.String(), nil
}

func parseVolumes(specs []ComposeVolume, baseDir string, declared map[string]Volume) ([]config.Mount, error) {
	mounts := make([]config.Mount, 0, len(specs))
	for i, spec := range specs {
		mount, ok, err := parseComposeVolume(spec, baseDir, declared)
		if err != nil {
			return nil, fmt.Errorf("volume %d: %w", i, err)
		}
		if !ok {
			continue
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func parseComposeVolume(spec ComposeVolume, baseDir string, declared map[string]Volume) (config.Mount, bool, error) {
	if spec.Short != "" {
		mount, err := parseVolume(spec.Short, baseDir, declared)
		return mount, true, err
	}
	typ := spec.Type
	if typ == "" {
		typ = "volume"
	}
	switch typ {
	case "bind", "volume":
		if spec.Source == "" || spec.Target == "" {
			return config.Mount{}, false, fmt.Errorf("%s volume requires source and target", typ)
		}
		mode := "rw"
		if spec.ReadOnly {
			mode = "ro"
		}
		mount, err := parseVolume(spec.Source+":"+spec.Target+":"+mode, baseDir, declared)
		return mount, true, err
	case "tmpfs", "image", "npipe", "cluster":
		return config.Mount{}, false, nil
	default:
		return config.Mount{}, false, fmt.Errorf("unsupported volume type %q", spec.Type)
	}
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
	readOnly := false
	if len(parts) == 3 {
		// Only `ro` is supported today. The previous implementation
		// accepted anything here and silently fell back to
		// read-write, so a typo like `:readonly`, `:r0`, or
		// docker-compose's `:rw,Z` got interpreted as "mount it
		// writable" and the operator had no signal that their
		// intent was dropped. Fail loudly instead; the day we grow
		// more modes (e.g. rshared, noexec, nodev) we add them to
		// this allow-list deliberately.
		switch parts[2] {
		case "ro":
			readOnly = true
		case "rw":
			readOnly = false
		default:
			return config.Mount{}, fmt.Errorf(
				"volume %q: unknown mode %q (supported: ro, rw)",
				spec, parts[2])
		}
	}

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
// plain bytes ("1048576"), or a decimal with a unit suffix: K/M/G/T with an
// optional B ("2G", "2GB"). Multipliers are binary, matching qemu-img
// convention. Empty returns the default.
func parseVolumeSize(raw string) (int64, error) {
	if raw == "" {
		return defaultVolumeSizeBytes, nil
	}

	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return defaultVolumeSizeBytes, nil
	}
	if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
		if s == "" {
			return 0, fmt.Errorf("invalid size %q (expected e.g. \"10GB\")", raw)
		}
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
		return 0, fmt.Errorf("invalid size %q (expected e.g. \"10GB\"): %w", raw, err)
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

	// Cached remote images are guaranteed to exist: images.Pull only
	// returns a cached path after stating it. Local-path refs
	// (images.Pull returns them verbatim when the ref is not a
	// registry entry) bypass that stat, so `holos validate` would
	// silently approve `image: ./missing.qcow2` and the real error
	// would not surface until qemu-img failed deep in `holos up`.
	// Checking here turns the silent pass into an early, specific
	// failure that names the compose field.
	if info, err := os.Stat(path); err != nil {
		return "", "", fmt.Errorf("image %q: %w", ref, err)
	} else if info.IsDir() {
		return "", "", fmt.Errorf("image %q is a directory, expected a qcow2 or raw file", ref)
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
