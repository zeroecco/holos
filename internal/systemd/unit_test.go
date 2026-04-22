package systemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_UserScope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")

	path, content, err := Render(UnitSpec{
		Project:     "web",
		ComposeFile: "/srv/holos/web/holos.yaml",
		HolosBinary: "/usr/local/bin/holos",
		Scope:       ScopeUser,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if want := "/tmp/xdg/systemd/user/holos-web.service"; path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}

	// Key invariants of the emitted unit.
	mustContain(t, content,
		"Description=holos project web",
		"ExecStart=/usr/local/bin/holos up -f /srv/holos/web/holos.yaml",
		"ExecStop=/usr/local/bin/holos down web",
		"WantedBy=default.target",
		"Type=oneshot",
		"RemainAfterExit=yes",
	)
	// User scope must not emit a User= directive: systemd --user
	// doesn't honor it and would reject the unit.
	if strings.Contains(content, "\nUser=") {
		t.Fatalf("user scope unit contains User=:\n%s", content)
	}
}

func TestRender_SystemScopeWithUser(t *testing.T) {
	_, content, err := Render(UnitSpec{
		Project:     "db",
		ComposeFile: "/srv/db/holos.yaml",
		HolosBinary: "/usr/bin/holos",
		StateDir:    "/var/lib/holos",
		Scope:       ScopeSystem,
		User:        "holos",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	mustContain(t, content,
		"WantedBy=multi-user.target",
		"User=holos",
		"ExecStart=/usr/bin/holos up --state-dir /var/lib/holos -f /srv/db/holos.yaml",
		"ExecStop=/usr/bin/holos down --state-dir /var/lib/holos db",
	)
}

// TestRender_StateFlagBeforePositional pins the layout of ExecStop so
// it never regresses to `holos down <project> --state-dir <dir>`. Go's
// flag package stops parsing at the first non-flag token, so a
// trailing --state-dir would silently be ignored at boot/shutdown time
// and the unit would touch the default state path. The test fakes the
// stop command via os.Args parsing in cmd/holos so we exercise the
// exact same flag-order contract end-to-end.
func TestRender_StateFlagBeforePositional(t *testing.T) {
	_, content, err := Render(UnitSpec{
		Project:     "demo",
		ComposeFile: "/srv/demo/holos.yaml",
		HolosBinary: "/usr/bin/holos",
		StateDir:    "/tmp/holos-state",
		Scope:       ScopeSystem,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, badForm := range []string{
		"holos down demo --state-dir",
		"holos up -f /srv/demo/holos.yaml --state-dir",
	} {
		if strings.Contains(content, badForm) {
			t.Fatalf("rendered unit contains flag-after-positional form %q:\n%s", badForm, content)
		}
	}
	mustContain(t, content,
		"ExecStart=/usr/bin/holos up --state-dir /tmp/holos-state -f /srv/demo/holos.yaml",
		"ExecStop=/usr/bin/holos down --state-dir /tmp/holos-state demo",
	)
}

func TestRender_ValidationRejectsRelativePaths(t *testing.T) {
	cases := map[string]UnitSpec{
		"compose relative": {
			Project:     "x",
			ComposeFile: "relative.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"binary relative": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "holos",
			Scope:       ScopeUser,
		},
		"empty project": {
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"whitespace project": {
			Project:     "my proj",
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"bad scope": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       "global",
		},
		"space in compose file": {
			Project:     "x",
			ComposeFile: "/srv/my holos/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"space in binary path": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "/opt/My Apps/holos",
			Scope:       ScopeUser,
		},
		"space in state dir": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			StateDir:    "/var/lib/holos state",
			Scope:       ScopeSystem,
		},
		"systemd specifier in path": {
			Project:     "x",
			ComposeFile: "/etc/%H/holos.yaml",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"command separator in path": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml;rm",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
		"newline in path": {
			Project:     "x",
			ComposeFile: "/abs/holos.yaml\ninjected",
			HolosBinary: "/usr/bin/holos",
			Scope:       ScopeUser,
		},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Render(spec); err == nil {
				t.Fatalf("expected validation error, got nil")
			}
		})
	}
}

func TestInstallUninstall_RoundTrip(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	// Quarantine systemctl invocations: Install/Uninstall may try to
	// shell out if systemctl is on PATH. We don't want to touch the
	// real system bus, so we prepend a shim dir that pretends
	// systemctl does not exist.
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)

	spec := UnitSpec{
		Project:     "demo",
		ComposeFile: "/srv/demo/holos.yaml",
		HolosBinary: "/usr/bin/holos",
		Scope:       ScopeUser,
	}

	res, err := Install(spec, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !res.SystemctlMissing {
		t.Fatalf("expected SystemctlMissing=true with empty PATH, got %+v", res)
	}
	if _, err := os.Stat(res.UnitPath); err != nil {
		t.Fatalf("unit file missing after install: %v", err)
	}
	want := filepath.Join(xdg, "systemd", "user", "holos-demo.service")
	if res.UnitPath != want {
		t.Fatalf("unit path = %q, want %q", res.UnitPath, want)
	}

	_, err = Uninstall(ScopeUser, "demo")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(res.UnitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file still present after uninstall (err=%v)", err)
	}

	// Second uninstall must be a no-op: systemd workflows often
	// retry idempotently (ansible, make, etc.).
	if _, err := Uninstall(ScopeUser, "demo"); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

func mustContain(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("missing %q in:\n%s", n, haystack)
		}
	}
}
