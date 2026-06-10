package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	yamlNullTag = "!!null"
	yamlSeqTag  = "!!seq"
	yamlMapTag  = "!!map"
)

// StringList accepts Docker Compose fields that allow either a scalar string
// or a list of scalar values.
type StringList []string

func (l *StringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == yamlNullTag {
			*l = nil
			return nil
		}
		*l = StringList{node.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			value, ok, err := decodeStringListItem(item)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			out = append(out, value)
		}
		*l = StringList(out)
		return nil
	default:
		return fmt.Errorf("line %d: value must be a string or list", node.Line)
	}
}

func decodeStringListItem(node *yaml.Node) (value string, ok bool, err error) {
	if node.Kind != yaml.ScalarNode {
		return "", false, fmt.Errorf("line %d: list entries must be scalar values", node.Line)
	}
	if isYAMLNullScalar(node) {
		return "", false, nil
	}
	return node.Value, true, nil
}

func isYAMLNullScalar(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == yamlNullTag
}

// ComposeStringMap accepts Compose fields that can be declared either as a
// mapping or as KEY=VALUE list entries.
type ComposeStringMap map[string]string

func (m *ComposeStringMap) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[key] = value
		}
		*m = ComposeStringMap(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			key, value, ok := parseComposeStringMapEntry(raw)
			if !ok {
				return fmt.Errorf("line %d: entries must use KEY=VALUE syntax", item.Line)
			}
			out[key] = value
		}
		*m = ComposeStringMap(out)
		return nil
	default:
		return fmt.Errorf("line %d: value must be a mapping or KEY=VALUE list", node.Line)
	}
}

func parseComposeStringMapEntry(raw string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(raw, envFileAssignmentToken)
	if !ok {
		return "", "", false
	}
	return key, value, true
}

// ComposeListOrMap accepts Docker Compose options fields that can be either a
// string list or a mapping. Holos does not interpret these fields today, but
// retaining their shape avoids rejecting valid Compose files during strict
// decode.
type ComposeListOrMap struct {
	List []string
	Map  map[string]any
}

func (m *ComposeListOrMap) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key, value, err := decodeListOrMapEntry(node.Content[i], node.Content[i+1])
			if err != nil {
				return err
			}
			out[key] = value
		}
		m.Map = out
		m.List = nil
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			value, err := decodeListOrMapListItem(item)
			if err != nil {
				return err
			}
			out = append(out, value)
		}
		m.List = out
		m.Map = nil
		return nil
	default:
		return fmt.Errorf("line %d: value must be a mapping or list", node.Line)
	}
}

func decodeListOrMapEntry(keyNode, valueNode *yaml.Node) (string, any, error) {
	var value any
	if err := valueNode.Decode(&value); err != nil {
		return "", nil, err
	}
	return keyNode.Value, value, nil
}

func decodeListOrMapListItem(node *yaml.Node) (string, error) {
	var value string
	if err := node.Decode(&value); err != nil {
		return "", err
	}
	return value, nil
}
