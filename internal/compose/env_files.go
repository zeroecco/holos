package compose

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// EnvFiles accepts Docker Compose env_file string, list, and mapping entries.
type EnvFiles []EnvFile

type EnvFile struct {
	Path     string `yaml:"path"`
	Required any    `yaml:"required,omitempty"`
	Format   string `yaml:"format,omitempty"`
}

const (
	envFilePathKey     = "path"
	envFileRequiredKey = "required"
	envFileFormatKey   = "format"
)

var envFileAllowedKeys = map[string]struct{}{
	envFilePathKey:     {},
	envFileRequiredKey: {},
	envFileFormatKey:   {},
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
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if !isEnvFileAllowedKey(key) {
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

func isEnvFileAllowedKey(key string) bool {
	_, ok := envFileAllowedKeys[key]
	return ok
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
