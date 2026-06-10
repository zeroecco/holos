package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
			name, err := decodeServiceNetworkListItem(item)
			if err != nil {
				return err
			}
			out[name] = ServiceNetwork{}
		}
		*n = out
		return nil
	case yaml.MappingNode:
		out := make(ServiceNetworks, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			network, err := decodeServiceNetworkMapValue(node.Content[i+1])
			if err != nil {
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

func decodeServiceNetworkMapValue(node *yaml.Node) (ServiceNetwork, error) {
	if isYAMLNullScalar(node) {
		return ServiceNetwork{}, nil
	}
	var network ServiceNetwork
	if err := node.Decode(&network); err != nil {
		return ServiceNetwork{}, err
	}
	return network, nil
}

func decodeServiceNetworkListItem(node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("line %d: network list entries must be scalar values", node.Line)
	}
	return node.Value, nil
}
