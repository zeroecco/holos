package compose

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

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

type composePortLongSyntax struct {
	Name        string `yaml:"name,omitempty"`
	Target      int    `yaml:"target"`
	Published   string `yaml:"published,omitempty"`
	HostIP      string `yaml:"host_ip,omitempty"`
	Protocol    string `yaml:"protocol,omitempty"`
	AppProtocol string `yaml:"app_protocol,omitempty"`
	Mode        string `yaml:"mode,omitempty"`
}

const (
	portFieldName        = "name"
	portFieldTarget      = "target"
	portFieldPublished   = "published"
	portFieldHostIP      = "host_ip"
	portFieldProtocol    = "protocol"
	portFieldAppProtocol = "app_protocol"
	portFieldMode        = "mode"
)

var composePortAllowedFields = map[string]struct{}{
	portFieldName:        {},
	portFieldTarget:      {},
	portFieldPublished:   {},
	portFieldHostIP:      {},
	portFieldProtocol:    {},
	portFieldAppProtocol: {},
	portFieldMode:        {},
}

func (p *ComposePort) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		p.Short = node.Value
		return nil
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if !isComposePortAllowedField(key) {
				return fmt.Errorf("line %d: field %s not found in type compose.ComposePort", node.Content[i].Line, key)
			}
			value := node.Content[i+1]
			switch key {
			case portFieldName:
				name, err := decodeComposePortString(value)
				if err != nil {
					return err
				}
				p.Name = name
			case portFieldTarget:
				target, err := decodeComposePortTarget(value)
				if err != nil {
					return err
				}
				p.Target = target
				p.hasTarget = true
			case portFieldPublished:
				published, err := decodeComposePortPublished(value)
				if err != nil {
					return err
				}
				p.Published = published
			case portFieldHostIP:
				hostIP, err := decodeComposePortString(value)
				if err != nil {
					return err
				}
				p.HostIP = hostIP
			case portFieldProtocol:
				protocol, err := decodeComposePortString(value)
				if err != nil {
					return err
				}
				p.Protocol = protocol
			case portFieldAppProtocol:
				appProtocol, err := decodeComposePortString(value)
				if err != nil {
					return err
				}
				p.AppProtocol = appProtocol
			case portFieldMode:
				mode, err := decodeComposePortString(value)
				if err != nil {
					return err
				}
				p.Mode = mode
			}
		}
		return nil
	default:
		return fmt.Errorf("line %d: ports entries must be a string or mapping", node.Line)
	}
}

func decodeComposePortTarget(node *yaml.Node) (int, error) {
	var target string
	if err := decodeComposeStringOrInt(node, &target); err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(target)
	if err != nil {
		return 0, fmt.Errorf("target: %w", err)
	}
	return parsed, nil
}

func decodeComposePortPublished(node *yaml.Node) (string, error) {
	var published string
	if err := decodeComposeStringOrInt(node, &published); err != nil {
		return "", fmt.Errorf("published: %w", err)
	}
	return published, nil
}

func decodeComposePortString(node *yaml.Node) (string, error) {
	var value string
	if err := node.Decode(&value); err != nil {
		return "", err
	}
	return value, nil
}

func isComposePortAllowedField(key string) bool {
	_, ok := composePortAllowedFields[key]
	return ok
}

func (p ComposePort) MarshalYAML() (any, error) {
	if p.Short != "" {
		return p.Short, nil
	}
	return composePortLongSyntax{
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
