package compose

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ComposeBuild accepts Docker Compose build string and mapping syntax. Holos
// translates context/dockerfile into the existing Dockerfile provisioning path;
// other build keys are retained as compatibility metadata.
type ComposeBuild struct {
	Context            string           `yaml:"context,omitempty"`
	Dockerfile         string           `yaml:"dockerfile,omitempty"`
	DockerfileInline   string           `yaml:"dockerfile_inline,omitempty"`
	Args               Environment      `yaml:"args,omitempty"`
	AdditionalContexts ComposeStringMap `yaml:"additional_contexts,omitempty"`
	CacheFrom          StringList       `yaml:"cache_from,omitempty"`
	CacheTo            StringList       `yaml:"cache_to,omitempty"`
	Entitlements       StringList       `yaml:"entitlements,omitempty"`
	ExtraHosts         ExtraHosts       `yaml:"extra_hosts,omitempty"`
	Isolation          string           `yaml:"isolation,omitempty"`
	Labels             ComposeLabels    `yaml:"labels,omitempty"`
	Network            string           `yaml:"network,omitempty"`
	NoCache            any              `yaml:"no_cache,omitempty"`
	Pull               any              `yaml:"pull,omitempty"`
	Provenance         any              `yaml:"provenance,omitempty"`
	SBOM               any              `yaml:"sbom,omitempty"`
	Secrets            ServiceResources `yaml:"secrets,omitempty"`
	ShmSize            any              `yaml:"shm_size,omitempty"`
	SSH                ComposeStringMap `yaml:"ssh,omitempty"`
	Tags               StringList       `yaml:"tags,omitempty"`
	Target             string           `yaml:"target,omitempty"`
	Ulimits            map[string]any   `yaml:"ulimits,omitempty"`
	Platforms          StringList       `yaml:"platforms,omitempty"`
	Privileged         any              `yaml:"privileged,omitempty"`
}

var composeBuildAllowedFields = map[string]struct{}{
	"context":             {},
	"dockerfile":          {},
	"dockerfile_inline":   {},
	"args":                {},
	"additional_contexts": {},
	"cache_from":          {},
	"cache_to":            {},
	"entitlements":        {},
	"extra_hosts":         {},
	"isolation":           {},
	"labels":              {},
	"network":             {},
	"no_cache":            {},
	"pull":                {},
	"provenance":          {},
	"sbom":                {},
	"secrets":             {},
	"shm_size":            {},
	"ssh":                 {},
	"tags":                {},
	"target":              {},
	"ulimits":             {},
	"platforms":           {},
	"privileged":          {},
}

func (b *ComposeBuild) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == yamlNullTag {
			return nil
		}
		b.Context = node.Value
		return nil
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if !isComposeBuildAllowedField(key) {
				return fmt.Errorf("line %d: field %s not found in type compose.ComposeBuild", node.Content[i].Line, key)
			}
		}
		type rawBuild ComposeBuild
		return node.Decode((*rawBuild)(b))
	default:
		return fmt.Errorf("line %d: build must be a string or mapping", node.Line)
	}
}

func isComposeBuildAllowedField(key string) bool {
	_, ok := composeBuildAllowedFields[key]
	return ok
}

func (b ComposeBuild) isSet() bool {
	return b.Context != "" || b.Dockerfile != "" || b.DockerfileInline != ""
}
