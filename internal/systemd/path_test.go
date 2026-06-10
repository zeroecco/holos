package systemd

import (
	"path/filepath"
	"testing"
)

func TestUnitPath(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		env   string
		want  string
	}{
		{
			name:  "system",
			scope: ScopeSystem,
			want:  "/etc/systemd/system/holos-demo.service",
		},
		{
			name:  "user XDG config home",
			scope: ScopeUser,
			env:   "/tmp/xdg",
			want:  filepath.FromSlash("/tmp/xdg/systemd/user/holos-demo.service"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("XDG_CONFIG_HOME", tt.env)
			}

			path, err := UnitPath(tt.scope, "demo")
			if err != nil {
				t.Fatalf("UnitPath: %v", err)
			}
			if path != tt.want {
				t.Fatalf("UnitPath = %q, want %q", path, tt.want)
			}
		})
	}
}
