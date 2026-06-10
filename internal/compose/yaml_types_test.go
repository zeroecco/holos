package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseComposeStringMapEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{name: "assignment", raw: "type=registry", wantKey: "type", wantValue: "registry", wantOK: true},
		{name: "keeps equals in value", raw: "ref=a=b", wantKey: "ref", wantValue: "a=b", wantOK: true},
		{name: "missing separator", raw: "flag", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, value, ok := parseComposeStringMapEntry(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey || value != tt.wantValue {
				t.Fatalf("parseComposeStringMapEntry(%q) = (%q, %q), want (%q, %q)",
					tt.raw, key, value, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestDecodeStringListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		node    yaml.Node
		want    string
		wantOK  bool
		wantErr string
	}{
		{name: "scalar", node: yaml.Node{Kind: yaml.ScalarNode, Value: "value"}, want: "value", wantOK: true},
		{name: "null", node: yaml.Node{Kind: yaml.ScalarNode, Tag: yamlNullTag}, wantOK: false},
		{name: "mapping", node: yaml.Node{Kind: yaml.MappingNode, Line: 7}, wantErr: "line 7: list entries must be scalar values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := decodeStringListItem(&tt.node)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeStringListItem: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("decodeStringListItem = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsYAMLNullScalar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node yaml.Node
		want bool
	}{
		{name: "null scalar", node: yaml.Node{Kind: yaml.ScalarNode, Tag: yamlNullTag}, want: true},
		{name: "string scalar", node: yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}, want: false},
		{name: "null sequence", node: yaml.Node{Kind: yaml.SequenceNode, Tag: yamlNullTag}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isYAMLNullScalar(&tt.node); got != tt.want {
				t.Fatalf("isYAMLNullScalar = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeListOrMapEntry(t *testing.T) {
	t.Parallel()

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`
enabled: true
limits:
  count: 2
`), &node); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}
	mapping := node.Content[0]

	key, value, err := decodeListOrMapEntry(mapping.Content[0], mapping.Content[1])
	if err != nil {
		t.Fatalf("decodeListOrMapEntry scalar: %v", err)
	}
	if key != "enabled" || value != true {
		t.Fatalf("scalar entry = (%q, %#v), want (enabled, true)", key, value)
	}

	key, value, err = decodeListOrMapEntry(mapping.Content[2], mapping.Content[3])
	if err != nil {
		t.Fatalf("decodeListOrMapEntry nested: %v", err)
	}
	nested, ok := value.(map[string]any)
	if key != "limits" || !ok || nested["count"] != 2 {
		t.Fatalf("nested entry = (%q, %#v), want limits map with count 2", key, value)
	}
}

func TestDecodeListOrMapListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "scalar", raw: "cache=type=registry", want: "cache=type=registry"},
		{name: "invalid", raw: "{cache: registry}", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal YAML: %v", err)
			}
			got, err := decodeListOrMapListItem(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeListOrMapListItem: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeListOrMapListItem = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeComposeYAMLNodeStripsExtensionFields(t *testing.T) {
	t.Parallel()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(`
x-root: ignored
services:
  vm:
    image: base.qcow2
    x-service: ignored
`), &doc); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}

	normalizeComposeYAMLNode(&doc)

	root := doc.Content[0]
	if mappingHasKey(root, "x-root") {
		t.Fatal("root extension key was not stripped")
	}
	services := mappingValue(root, "services")
	vm := mappingValue(mappingValue(services, "vm"), "x-service")
	if vm != nil {
		t.Fatal("nested extension key was not stripped")
	}
	if mappingValue(mappingValue(services, "vm"), "image") == nil {
		t.Fatal("non-extension key was stripped")
	}
}

func TestApplyComposeYAMLTag(t *testing.T) {
	t.Parallel()

	override := yaml.Node{Kind: yaml.MappingNode, Tag: composeOverrideTag}
	if done := applyComposeYAMLTag(&override); done {
		t.Fatal("applyComposeYAMLTag override done = true, want false")
	}
	if override.Tag != "" {
		t.Fatalf("override tag = %q, want empty", override.Tag)
	}

	reset := yaml.Node{Kind: yaml.MappingNode, Tag: composeResetTag, Content: []*yaml.Node{{Kind: yaml.ScalarNode}}}
	if done := applyComposeYAMLTag(&reset); !done {
		t.Fatal("applyComposeYAMLTag reset done = false, want true")
	}
	if reset.Kind != yaml.MappingNode || reset.Tag != yamlMapTag || len(reset.Content) != 0 {
		t.Fatalf("reset node = kind %v tag %q content %d, want empty mapping", reset.Kind, reset.Tag, len(reset.Content))
	}
}

func TestNormalizeYAMLAlias(t *testing.T) {
	t.Parallel()

	nilAlias := yaml.Node{Kind: yaml.AliasNode}
	normalizeYAMLAlias(&nilAlias)
	if nilAlias.Kind != yaml.AliasNode {
		t.Fatalf("nil alias kind = %v, want alias", nilAlias.Kind)
	}

	target := yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "x-debug"},
			{Kind: yaml.ScalarNode, Value: "ignored"},
			{Kind: yaml.ScalarNode, Value: "image"},
			{Kind: yaml.ScalarNode, Value: "base.qcow2"},
		},
	}
	alias := yaml.Node{Kind: yaml.AliasNode, Alias: &target}
	normalizeYAMLAlias(&alias)
	if alias.Kind != yaml.MappingNode {
		t.Fatalf("alias kind = %v, want mapping", alias.Kind)
	}
	if mappingHasKey(&alias, "x-debug") {
		t.Fatal("alias normalization retained extension key")
	}
	if got := mappingValue(&alias, "image"); got == nil || got.Value != "base.qcow2" {
		t.Fatalf("alias image = %+v, want base.qcow2", got)
	}
	if len(target.Content) != 4 {
		t.Fatalf("alias normalization mutated target content len = %d, want 4", len(target.Content))
	}
}

func TestResetYAMLNodeClearsByNodeKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		node     yaml.Node
		wantKind yaml.Kind
		wantTag  string
	}{
		{name: "sequence", node: yaml.Node{Kind: yaml.SequenceNode, Tag: yamlSeqTag, Content: []*yaml.Node{{Kind: yaml.ScalarNode}}}, wantKind: yaml.SequenceNode, wantTag: yamlSeqTag},
		{name: "mapping", node: yaml.Node{Kind: yaml.MappingNode, Tag: yamlMapTag, Content: []*yaml.Node{{Kind: yaml.ScalarNode}, {Kind: yaml.ScalarNode}}}, wantKind: yaml.MappingNode, wantTag: yamlMapTag},
		{name: "scalar", node: yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "value"}, wantKind: yaml.ScalarNode, wantTag: yamlNullTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resetYAMLNode(&tt.node)
			if tt.node.Kind != tt.wantKind || tt.node.Tag != tt.wantTag {
				t.Fatalf("reset node kind/tag = %v/%q, want %v/%q", tt.node.Kind, tt.node.Tag, tt.wantKind, tt.wantTag)
			}
			if len(tt.node.Content) != 0 {
				t.Fatalf("reset node content len = %d, want 0", len(tt.node.Content))
			}
			if tt.name == "scalar" && tt.node.Value != "" {
				t.Fatalf("reset scalar value = %q, want empty", tt.node.Value)
			}
		})
	}
}

func mappingHasKey(node *yaml.Node, key string) bool {
	return mappingValue(node, key) != nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
