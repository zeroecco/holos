package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
			value, err := decodeEnvironmentMapValue(node.Content[i+1])
			if err != nil {
				return err
			}
			out[key] = value
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
			key, value := parseEnvironmentEntry(raw)
			if value == nil {
				out[key] = nil
				continue
			}
			out[key] = value
		}
		*e = Environment(out)
		return nil
	default:
		return fmt.Errorf("line %d: environment must be a mapping or list", node.Line)
	}
}

func decodeEnvironmentMapValue(node *yaml.Node) (*string, error) {
	if node.Kind == yaml.ScalarNode && node.Tag == yamlNullTag {
		return nil, nil
	}
	var raw string
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}
	return stringPtr(raw), nil
}

func parseEnvironmentEntry(raw string) (key string, value *string) {
	key, rawValue, ok := strings.Cut(raw, envFileAssignmentToken)
	if !ok {
		return raw, nil
	}
	return key, stringPtr(rawValue)
}

func stringPtr(s string) *string {
	return &s
}
