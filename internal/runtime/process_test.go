package runtime

import (
	"testing"
)

func TestIsQEMUProcessName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		comm string
		want bool
	}{
		{name: "qemu system newline", comm: qemuSystemDefault + "\n", want: true},
		{name: "qemu system whitespace", comm: " " + qemuSystemDefault + " \n", want: true},
		{name: "qemu img", comm: qemuImgDefault, want: true},
		{name: "shell", comm: "bash\n"},
		{name: "empty", comm: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isQEMUProcessName(tt.comm); got != tt.want {
				t.Fatalf("isQEMUProcessName(%q) = %v, want %v", tt.comm, got, tt.want)
			}
		})
	}
}
