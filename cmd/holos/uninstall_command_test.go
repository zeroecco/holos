package main

import (
	"bytes"
	"testing"

	"github.com/zeroecco/holos/internal/systemd"
)

func TestWriteUninstallResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		res      systemd.Result
		wantOut  string
		wantWarn string
	}{
		{
			name:    "removed user unit",
			res:     systemd.Result{Scope: systemd.ScopeUser, UnitPath: "/home/me/.config/systemd/user/holos-demo.service"},
			wantOut: "removed user unit: /home/me/.config/systemd/user/holos-demo.service\n",
		},
		{
			name: "warnings go to warning output",
			res: systemd.Result{
				Scope:    systemd.ScopeSystem,
				UnitPath: "/etc/systemd/system/holos-demo.service",
				Warnings: []string{"disable: exit status 1"},
			},
			wantOut:  "removed system unit: /etc/systemd/system/holos-demo.service\n",
			wantWarn: "warning: disable: exit status 1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			var warnings bytes.Buffer
			writeUninstallResult(&out, &warnings, tt.res)
			if got := out.String(); got != tt.wantOut {
				t.Fatalf("uninstall output = %q, want %q", got, tt.wantOut)
			}
			if got := warnings.String(); got != tt.wantWarn {
				t.Fatalf("uninstall warnings = %q, want %q", got, tt.wantWarn)
			}
		})
	}
}
