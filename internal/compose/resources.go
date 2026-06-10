package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
		ref, err := decodeServiceResource(item)
		if err != nil {
			return err
		}
		out = append(out, ref)
	}
	*r = ServiceResources(out)
	return nil
}

func decodeServiceResource(node *yaml.Node) (ServiceResource, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return ServiceResource{Source: node.Value}, nil
	case yaml.MappingNode:
		var ref ServiceResource
		if err := node.Decode(&ref); err != nil {
			return ServiceResource{}, err
		}
		return ref, nil
	default:
		return ServiceResource{}, fmt.Errorf("line %d: resource references must be strings or mappings", node.Line)
	}
}
