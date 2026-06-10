package config

import "testing"

func TestInferImageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want string
	}{
		{path: "disk.raw", want: ImageFormatRaw},
		{path: "disk.img", want: ImageFormatRaw},
		{path: "disk.qcow2", want: ImageFormatQCOW2},
		{path: "disk", want: ImageFormatQCOW2},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			if got := inferImageFormat(tt.path); got != tt.want {
				t.Fatalf("inferImageFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
