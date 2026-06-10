package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestServiceModelsUnmarshalList(t *testing.T) {
	t.Parallel()

	var models ServiceModels
	if err := yaml.Unmarshal([]byte("- llm\n- embeddings\n"), &models); err != nil {
		t.Fatalf("unmarshal service models: %v", err)
	}
	if _, ok := models["llm"]; !ok {
		t.Fatalf("models missing llm entry: %#v", models)
	}
	if _, ok := models["embeddings"]; !ok {
		t.Fatalf("models missing embeddings entry: %#v", models)
	}
}

func TestDecodeServiceModelListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		node    yaml.Node
		want    string
		wantErr string
	}{
		{name: "scalar", node: yaml.Node{Kind: yaml.ScalarNode, Value: "llm"}, want: "llm"},
		{name: "mapping", node: yaml.Node{Kind: yaml.MappingNode, Line: 9}, wantErr: "line 9: model list entries must be scalar values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeServiceModelListItem(&tt.node)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeServiceModelListItem: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeServiceModelListItem = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceModelsUnmarshalNullMappingValue(t *testing.T) {
	t.Parallel()

	var models ServiceModels
	if err := yaml.Unmarshal([]byte("llm:\nembeddings:\n  endpoint_var: EMBED_URL\n"), &models); err != nil {
		t.Fatalf("unmarshal service models: %v", err)
	}
	if got := models["llm"]; got != (ServiceModel{}) {
		t.Fatalf("models[llm] = %#v, want empty ServiceModel", got)
	}
	if got := models["embeddings"].EndpointVar; got != "EMBED_URL" {
		t.Fatalf("models[embeddings].EndpointVar = %q, want EMBED_URL", got)
	}
}

func TestDecodeServiceModelMapValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantVar     string
		wantDefault bool
		wantErr     string
	}{
		{name: "null", raw: "null", wantDefault: true},
		{name: "mapping", raw: "endpoint_var: LLM_URL", wantVar: "LLM_URL"},
		{name: "invalid", raw: "llm", wantErr: "cannot unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var node yaml.Node
			if err := yaml.Unmarshal([]byte(tt.raw), &node); err != nil {
				t.Fatalf("unmarshal YAML: %v", err)
			}
			got, err := decodeServiceModelMapValue(node.Content[0])
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeServiceModelMapValue: %v", err)
			}
			if tt.wantDefault {
				if got != (ServiceModel{}) {
					t.Fatalf("decodeServiceModelMapValue = %+v, want zero value", got)
				}
				return
			}
			if got.EndpointVar != tt.wantVar {
				t.Fatalf("EndpointVar = %q, want %q", got.EndpointVar, tt.wantVar)
			}
		})
	}
}

func TestServiceModelsRejectsNonScalarListEntry(t *testing.T) {
	t.Parallel()

	var models ServiceModels
	err := yaml.Unmarshal([]byte("- name: llm\n"), &models)
	assertErrorContains(t, err, "model list entries must be scalar values")
}
