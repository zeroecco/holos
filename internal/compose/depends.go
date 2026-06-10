package compose

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// DependsOn accepts Docker Compose's short list syntax and long mapping
// syntax. Holos resolves both to a deterministic service-name list used for
// topological ordering; long-form options are accepted for Compose-file
// compatibility.
type DependsOn []string

const (
	dependsOnConditionKey = "condition"
	dependsOnRestartKey   = "restart"
	dependsOnRequiredKey  = "required"

	dependsConditionStarted               = "service_started"
	dependsConditionHealthy               = "service_healthy"
	dependsConditionCompletedSuccessfully = "service_completed_successfully"
)

var dependsOnAllowedFields = map[string]struct{}{
	dependsOnConditionKey: {},
	dependsOnRestartKey:   {},
	dependsOnRequiredKey:  {},
}

func (d *DependsOn) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var names []string
		if err := node.Decode(&names); err != nil {
			return err
		}
		*d = DependsOn(names)
		return nil
	case yaml.MappingNode:
		names := make([]string, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			name := node.Content[i].Value
			value := node.Content[i+1]
			if value.Kind == yaml.MappingNode {
				for j := 0; j < len(value.Content); j += 2 {
					key := value.Content[j].Value
					if !isDependsOnAllowedField(key) {
						return fmt.Errorf("line %d: field %s not found in type compose.DependsOn", value.Content[j].Line, key)
					}
				}
				var opts struct {
					Condition string `yaml:"condition"`
					Restart   *bool  `yaml:"restart"`
					Required  *bool  `yaml:"required"`
				}
				if err := value.Decode(&opts); err != nil {
					return err
				}
				switch opts.Condition {
				case "", dependsConditionStarted, dependsConditionHealthy, dependsConditionCompletedSuccessfully:
				default:
					return fmt.Errorf("line %d: unsupported depends_on condition %q", value.Line, opts.Condition)
				}
			} else if value.Kind != yaml.ScalarNode || value.Tag != yamlNullTag {
				return fmt.Errorf("line %d: depends_on service %q must be a mapping", value.Line, name)
			}
			names = append(names, name)
		}
		sort.Strings(names)
		*d = DependsOn(names)
		return nil
	default:
		return fmt.Errorf("line %d: depends_on must be a list or mapping", node.Line)
	}
}

func isDependsOnAllowedField(key string) bool {
	_, ok := dependsOnAllowedFields[key]
	return ok
}

func (d DependsOn) MarshalYAML() (any, error) {
	return []string(d), nil
}
