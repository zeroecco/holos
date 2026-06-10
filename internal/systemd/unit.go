// Package systemd generates and installs systemd unit files that
// bring holos projects back up after a host reboot.
//
// Two installation scopes are supported:
//
//   - "user" (default): drops the unit under $XDG_CONFIG_HOME/systemd/user
//     so it runs as the invoking user, with no root required. This is
//     the right choice for personal workstations and mirrors what
//     `podman generate systemd --user` does.
//   - "system": drops the unit under /etc/systemd/system so the host
//     brings the project up even before anyone logs in. This requires
//     write access to that directory (i.e. sudo / root).
//
// The generated unit is intentionally minimal: Type=oneshot plus
// RemainAfterExit so systemctl reports the service as "active" while
// the VMs themselves stay daemonized under holos's own state tracking.
// ExecStart runs `holos up [--state-dir D] -f <abs compose>`,
// ExecStop runs `holos down [--state-dir D] <project>`; flags are
// always rendered before any positional argument because Go's flag
// parser stops at the first non-flag token, which would otherwise
// drop --state-dir on stop. This keeps stop/start semantics identical
// to what the operator would type at the CLI.
package systemd

// Scope selects between a per-user unit and a host-wide unit.
type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
)

// UnitSpec describes the unit we want to emit. All paths must be
// absolute: systemd does not expand $HOME or relative paths at boot.
type UnitSpec struct {
	// Project is the holos project name; used to compute the unit
	// filename (holos-<project>.service) and in log descriptions.
	Project string

	// ComposeFile is the absolute path to the holos.yaml the unit
	// should bring up at boot.
	ComposeFile string

	// HolosBinary is the absolute path to the holos executable.
	// We resolve this explicitly so the unit keeps working even if
	// the operator's PATH changes or the binary is moved. At boot,
	// systemd units run with an almost-empty PATH; a bare "holos"
	// would simply fail.
	HolosBinary string

	// StateDir, if non-empty, is passed as --state-dir to every
	// holos invocation in the unit. Useful for tests and for
	// non-standard deployments.
	StateDir string

	// Scope selects user vs system installation.
	Scope Scope

	// User, when set with ScopeSystem, is written as User= in the
	// unit so the VMs run under that account (and its cached state
	// directory) rather than root. Ignored for user scope.
	User string
}
