package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
