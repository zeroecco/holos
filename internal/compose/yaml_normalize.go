package compose

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	composeOverrideTag     = "!override"
	composeResetTag        = "!reset"
	composeExtensionPrefix = "x-"
)

func normalizeComposeYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}
	if applyComposeYAMLTag(node) {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			normalizeComposeYAMLNode(child)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			normalizeComposeYAMLNode(child)
		}
	case yaml.MappingNode:
		out := node.Content[:0]
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if key.Kind == yaml.ScalarNode && strings.HasPrefix(key.Value, composeExtensionPrefix) {
				continue
			}
			normalizeComposeYAMLNode(value)
			out = append(out, key, value)
		}
		node.Content = out
	case yaml.AliasNode:
		normalizeYAMLAlias(node)
	}
}

func applyComposeYAMLTag(node *yaml.Node) (done bool) {
	switch node.Tag {
	case composeOverrideTag:
		node.Tag = ""
		return false
	case composeResetTag:
		resetYAMLNode(node)
		return true
	default:
		return false
	}
}

func cloneYAMLNode(node *yaml.Node) yaml.Node {
	if node == nil {
		return yaml.Node{}
	}
	clone := *node
	if len(node.Content) > 0 {
		clone.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			childClone := cloneYAMLNode(child)
			clone.Content[i] = &childClone
		}
	}
	if node.Alias != nil {
		aliasClone := cloneYAMLNode(node.Alias)
		clone.Alias = &aliasClone
	}
	return clone
}

func normalizeYAMLAlias(node *yaml.Node) {
	if node.Alias == nil {
		return
	}
	*node = cloneYAMLNode(node.Alias)
	normalizeComposeYAMLNode(node)
}

func resetYAMLNode(node *yaml.Node) {
	switch node.Kind {
	case yaml.SequenceNode:
		node.Tag = yamlSeqTag
		node.Content = nil
	case yaml.MappingNode:
		node.Tag = yamlMapTag
		node.Content = nil
	default:
		node.Kind = yaml.ScalarNode
		node.Tag = yamlNullTag
		node.Value = ""
		node.Content = nil
	}
}
