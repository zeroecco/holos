package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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

type composeVolumeLongSyntax struct {
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

func (v *ComposeVolume) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		v.Short = node.Value
		return nil
	case yaml.MappingNode:
		var long composeVolumeLongSyntax
		if err := node.Decode(&long); err != nil {
			return err
		}
		*v = composeVolumeFromLongSyntax(long)
		return nil
	default:
		return fmt.Errorf("line %d: volume entries must be strings or mappings", node.Line)
	}
}

func (v ComposeVolume) MarshalYAML() (any, error) {
	if v.Short != "" {
		return v.Short, nil
	}
	return v.longSyntax(), nil
}

func composeVolumeFromLongSyntax(long composeVolumeLongSyntax) ComposeVolume {
	return ComposeVolume{
		Type:        long.Type,
		Source:      long.Source,
		Target:      long.Target,
		ReadOnly:    long.ReadOnly,
		Consistency: long.Consistency,
		Bind:        long.Bind,
		Volume:      long.Volume,
		Tmpfs:       long.Tmpfs,
		Image:       long.Image,
	}
}

func (v ComposeVolume) longSyntax() composeVolumeLongSyntax {
	return composeVolumeLongSyntax{
		Type:        v.Type,
		Source:      v.Source,
		Target:      v.Target,
		ReadOnly:    v.ReadOnly,
		Consistency: v.Consistency,
		Bind:        v.Bind,
		Volume:      v.Volume,
		Tmpfs:       v.Tmpfs,
		Image:       v.Image,
	}
}
