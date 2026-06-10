package main

import (
	"bytes"
	"testing"

	"github.com/zeroecco/holos/internal/systemd"
)

func TestInstallScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		system bool
		want   systemd.Scope
	}{
		{name: "user", system: false, want: systemd.ScopeUser},
		{name: "system", system: true, want: systemd.ScopeSystem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installScope(tt.system); got != tt.want {
				t.Fatalf("installScope(%v) = %q, want %q", tt.system, got, tt.want)
			}
		})
	}
}

func TestInstallSystemUserRequiresExplicitStateDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		system           bool
		runAs            string
		stateDirExplicit bool
		want             bool
	}{
		{name: "system user without state dir", system: true, runAs: "holos", want: true},
		{name: "system user with state dir", system: true, runAs: "holos", stateDirExplicit: true},
		{name: "system default user", system: true},
		{name: "user unit", runAs: "holos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := installSystemUserRequiresExplicitStateDir(tt.system, tt.runAs, tt.stateDirExplicit)
			if got != tt.want {
				t.Fatalf("installSystemUserRequiresExplicitStateDir(%v, %q, %v) = %v, want %v",
					tt.system, tt.runAs, tt.stateDirExplicit, got, tt.want)
			}
		})
	}
}

func TestInstallSystemUserStateDirError(t *testing.T) {
	t.Parallel()

	err := installSystemUserStateDirError("holos")
	assertErrorContains(t, err, "install --system --user holos requires --state-dir")
	assertErrorContains(t, err, "0700")
}

func TestWriteInstallDryRun(t *testing.T) {
	t.Parallel()

	unitPath := "/home/me/.config/systemd/user/holos-demo.service"
	unitContent := "[Unit]\nDescription=demo\n"

	var out bytes.Buffer
	writeInstallDryRun(&out, unitPath, unitContent)
	want := "# would write to: /home/me/.config/systemd/user/holos-demo.service\n" +
		"[Unit]\n" +
		"Description=demo\n"
	if got := out.String(); got != want {
		t.Fatalf("install dry-run output = %q, want %q", got, want)
	}
}

func TestWriteInstallResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		res      systemd.Result
		enable   bool
		project  string
		wantOut  string
		wantWarn string
	}{
		{
			name:    "user unit with activation guidance",
			res:     systemd.Result{Scope: systemd.ScopeUser, UnitPath: "/home/me/.config/systemd/user/holos-demo.service"},
			project: "demo",
			wantOut: "installed user unit: /home/me/.config/systemd/user/holos-demo.service\n" +
				"to activate at boot: systemctl --user enable --now holos-demo.service\n",
		},
		{
			name:    "system unit with activation guidance",
			res:     systemd.Result{Scope: systemd.ScopeSystem, UnitPath: "/etc/systemd/system/holos-demo.service"},
			project: "demo",
			wantOut: "installed system unit: /etc/systemd/system/holos-demo.service\n" +
				"to activate at boot: sudo systemctl enable --now holos-demo.service\n",
		},
		{
			name:    "enabled omits activation guidance",
			res:     systemd.Result{Scope: systemd.ScopeSystem, UnitPath: "/etc/systemd/system/holos-demo.service"},
			enable:  true,
			project: "demo",
			wantOut: "installed system unit: /etc/systemd/system/holos-demo.service\n",
		},
		{
			name: "missing systemctl",
			res: systemd.Result{
				Scope:            systemd.ScopeUser,
				UnitPath:         "/home/me/.config/systemd/user/holos-demo.service",
				SystemctlMissing: true,
			},
			project: "demo",
			wantOut: "installed user unit: /home/me/.config/systemd/user/holos-demo.service\n" +
				"note: systemctl not found on PATH; unit is on disk but not loaded\n",
		},
		{
			name: "warnings go to warning output",
			res: systemd.Result{
				Scope:    systemd.ScopeUser,
				UnitPath: "/home/me/.config/systemd/user/holos-demo.service",
				Warnings: []string{"daemon-reload: exit status 1"},
			},
			project: "demo",
			wantOut: "installed user unit: /home/me/.config/systemd/user/holos-demo.service\n" +
				"to activate at boot: systemctl --user enable --now holos-demo.service\n",
			wantWarn: "warning: daemon-reload: exit status 1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			var warnings bytes.Buffer
			writeInstallResult(&out, &warnings, tt.res, tt.enable, tt.project)
			if got := out.String(); got != tt.wantOut {
				t.Fatalf("install output = %q, want %q", got, tt.wantOut)
			}
			if got := warnings.String(); got != tt.wantWarn {
				t.Fatalf("install warnings = %q, want %q", got, tt.wantWarn)
			}
		})
	}
}
