package compose

import (
	"fmt"
	"regexp"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateName enforces that a project, service, or volume name
// matches the DNS-label pattern used throughout holos.
//
// This is the single source of truth for "is this string safe to
// embed in a filesystem path or a systemd unit filename". The CLI
// funnels every user-supplied name (from `-f`, from positional
// arguments to `down`, `console`, `exec`, `logs`, and from
// `--name` on install/uninstall) through this helper so a value
// like "../../../etc/passwd" cannot be turned into a path like
// <state-dir>/projects/../../../etc/passwd.json or
// /etc/systemd/system/holos-../../etc/passwd.service.
//
// The pattern allows:
//   - 1 to 63 characters (DNS-label maximum)
//   - lowercase letters, digits, and hyphens
//   - first and last characters are alphanumeric
//
// That ruleset rejects path separators, path traversals (`..`),
// whitespace, control characters, shell metacharacters, and
// uppercase (which systemd treats case-insensitively on some
// filesystems, confusing `holos ps` output).
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name %q must match %s", name, namePattern.String())
	}
	return nil
}
