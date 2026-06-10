package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

type testComposeVolumeLongWant struct {
	volumeType string
	source     string
	target     string
	readOnly   bool
}

func assertComposeVolumeLongFields(t *testing.T, got ComposeVolume, want testComposeVolumeLongWant) {
	t.Helper()

	if got.Type != want.volumeType ||
		got.Source != want.source ||
		got.Target != want.target ||
		got.ReadOnly != want.readOnly {
		t.Fatalf("volume long fields = %+v, want %+v", got, want)
	}
}

func assertComposeVolumeLongSyntax(t *testing.T, got composeVolumeLongSyntax, want testComposeVolumeLongWant) {
	t.Helper()

	if got.Type != want.volumeType ||
		got.Source != want.source ||
		got.Target != want.target ||
		got.ReadOnly != want.readOnly {
		t.Fatalf("MarshalYAML long syntax = %+v, want %+v", got, want)
	}
}

func TestComposeVolumeUnmarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("short syntax", func(t *testing.T) {
		t.Parallel()

		var got ComposeVolume
		if err := yaml.Unmarshal([]byte(`./data:/mnt:ro`), &got); err != nil {
			t.Fatalf("UnmarshalYAML: %v", err)
		}
		if got.Short != "./data:/mnt:ro" {
			t.Fatalf("Short = %q, want short syntax", got.Short)
		}
	})

	t.Run("long syntax", func(t *testing.T) {
		t.Parallel()

		var got ComposeVolume
		if err := yaml.Unmarshal([]byte(`
type: bind
source: ./data
target: /mnt
read_only: true
`), &got); err != nil {
			t.Fatalf("UnmarshalYAML: %v", err)
		}
		assertComposeVolumeLongFields(t, got, testComposeVolumeLongWant{
			volumeType: composeVolumeTypeBind,
			source:     "./data",
			target:     "/mnt",
			readOnly:   true,
		})
	})

	t.Run("invalid shape", func(t *testing.T) {
		t.Parallel()

		var got ComposeVolume
		err := yaml.Unmarshal([]byte(`[./data:/mnt]`), &got)
		assertErrorContains(t, err, "volume entries must be strings or mappings")
	})
}

func TestComposeVolumeMarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("short syntax", func(t *testing.T) {
		t.Parallel()

		got, err := (ComposeVolume{Short: "./data:/mnt:ro"}).MarshalYAML()
		if err != nil {
			t.Fatalf("MarshalYAML: %v", err)
		}
		if got != "./data:/mnt:ro" {
			t.Fatalf("MarshalYAML = %#v, want short syntax string", got)
		}
	})

	t.Run("long syntax", func(t *testing.T) {
		t.Parallel()

		got, err := (ComposeVolume{
			Type:     composeVolumeTypeBind,
			Source:   "./data",
			Target:   "/mnt",
			ReadOnly: true,
		}).MarshalYAML()
		if err != nil {
			t.Fatalf("MarshalYAML: %v", err)
		}
		long, ok := got.(composeVolumeLongSyntax)
		if !ok {
			t.Fatalf("MarshalYAML type = %T, want composeVolumeLongSyntax", got)
		}
		assertComposeVolumeLongSyntax(t, long, testComposeVolumeLongWant{
			volumeType: composeVolumeTypeBind,
			source:     "./data",
			target:     "/mnt",
			readOnly:   true,
		})
	})
}
