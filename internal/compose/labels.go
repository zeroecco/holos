package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type ComposeLabels map[string]string

func (l *ComposeLabels) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]string, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			var value string
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			out[node.Content[i].Value] = value
		}
		*l = ComposeLabels(out)
		return nil
	case yaml.SequenceNode:
		out := make(map[string]string, len(node.Content))
		for _, item := range node.Content {
			var raw string
			if err := item.Decode(&raw); err != nil {
				return err
			}
			key, value := parseComposeLabel(raw)
			out[key] = value
		}
		*l = ComposeLabels(out)
		return nil
	default:
		return fmt.Errorf("line %d: labels must be a mapping or list", node.Line)
	}
}

func parseComposeLabel(raw string) (key, value string) {
	key, value, ok := strings.Cut(raw, envFileAssignmentToken)
	if !ok {
		return raw, ""
	}
	return key, value
}

func resolveLabels(baseDir string, files StringList, inline ComposeLabels) (map[string]string, error) {
	out := map[string]string{}
	for _, file := range files {
		path := resolveEnvFilePath(baseDir, file)
		labels, err := readEnvFile(path)
		if err != nil {
			return nil, fmt.Errorf("label_file %q: %w", file, err)
		}
		mergeLabelFileValues(out, labels)
	}
	for key, value := range inline {
		out[key] = value
	}
	return out, nil
}

func mergeLabelFileValues(out map[string]string, labels Environment) {
	for key, value := range labels {
		out[key] = labelFileValue(value)
	}
}

func labelFileValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
