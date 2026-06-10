package systemd

import (
	"slices"
	"testing"
)

func assertStringSliceEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestSystemctlArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "reload system", got: systemctlReloadArgs(ScopeSystem), want: []string{"daemon-reload"}},
		{name: "reload user", got: systemctlReloadArgs(ScopeUser), want: []string{"--user", "daemon-reload"}},
		{
			name: "enable system",
			got:  systemctlEnableArgs(ScopeSystem, "/etc/systemd/system/holos-demo.service", false),
			want: []string{"enable", "/etc/systemd/system/holos-demo.service"},
		},
		{
			name: "enable user now",
			got:  systemctlEnableArgs(ScopeUser, "/home/rich/.config/systemd/user/holos-demo.service", true),
			want: []string{"--user", "enable", "--now", "/home/rich/.config/systemd/user/holos-demo.service"},
		},
		{
			name: "disable user now uses unit basename",
			got:  systemctlDisableArgs(ScopeUser, "/home/rich/.config/systemd/user/holos-demo.service"),
			want: []string{"--user", "disable", "--now", "holos-demo.service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assertStringSliceEqual(t, "systemctl args", tt.got, tt.want)
		})
	}
}
