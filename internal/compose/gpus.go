package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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

const gpusAllValue = "all"

func (g *GPUs) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		all, err := decodeGPUsScalar(node)
		if err != nil {
			return err
		}
		g.All = all
		return nil
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

func decodeGPUsScalar(node *yaml.Node) (bool, error) {
	if node.Value == gpusAllValue {
		return true, nil
	}
	return false, fmt.Errorf("line %d: gpus scalar must be %q", node.Line, gpusAllValue)
}

func (g GPUs) MarshalYAML() (any, error) {
	if g.All {
		return gpusAllValue, nil
	}
	return g.Requests, nil
}
