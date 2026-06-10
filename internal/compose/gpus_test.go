package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGPUsUnmarshalAllScalar(t *testing.T) {
	t.Parallel()

	var gpus GPUs
	if err := yaml.Unmarshal([]byte(gpusAllValue), &gpus); err != nil {
		t.Fatalf("unmarshal gpus: %v", err)
	}
	if !gpus.All {
		t.Fatalf("gpus.All = false, want true")
	}
}

func TestDecodeGPUsScalar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		node    yaml.Node
		want    bool
		wantErr string
	}{
		{name: "all", node: yaml.Node{Kind: yaml.ScalarNode, Value: gpusAllValue}, want: true},
		{name: "unsupported", node: yaml.Node{Kind: yaml.ScalarNode, Value: "some", Line: 4}, wantErr: `line 4: gpus scalar must be "all"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeGPUsScalar(&tt.node)
			if tt.wantErr != "" {
				assertErrorContains(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("decodeGPUsScalar: %v", err)
			}
			if got != tt.want {
				t.Fatalf("decodeGPUsScalar = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGPUsRejectsUnsupportedScalar(t *testing.T) {
	t.Parallel()

	var gpus GPUs
	err := yaml.Unmarshal([]byte("some"), &gpus)
	assertErrorContains(t, err, `gpus scalar must be "all"`)
}
