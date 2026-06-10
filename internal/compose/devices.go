package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
