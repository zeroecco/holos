package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
			name, err := decodeServiceModelListItem(item)
			if err != nil {
				return err
			}
			out[name] = ServiceModel{}
		}
		*m = out
		return nil
	case yaml.MappingNode:
		out := make(ServiceModels, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			model, err := decodeServiceModelMapValue(node.Content[i+1])
			if err != nil {
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

func decodeServiceModelMapValue(node *yaml.Node) (ServiceModel, error) {
	if isYAMLNullScalar(node) {
		return ServiceModel{}, nil
	}
	var model ServiceModel
	if err := node.Decode(&model); err != nil {
		return ServiceModel{}, err
	}
	return model, nil
}

func decodeServiceModelListItem(node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("line %d: model list entries must be scalar values", node.Line)
	}
	return node.Value, nil
}
