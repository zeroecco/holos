package systemd

import (
	"errors"
	"fmt"
	"regexp"
)

// projectNamePattern mirrors compose.ValidateName; duplicated here so
// this package stays free of compose/config imports. UnitPath and
// Render call this defensively so that any caller slipping a bad
// project name past the CLI validator still cannot escape the unit
// directory.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name is empty")
	}
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf("project %q must match %s", name, projectNamePattern.String())
	}
	return nil
}

// usernamePattern is the portable POSIX "name" pattern (NAME_REGEX in
// /etc/adduser.conf on most distros): a leading lowercase letter or
// underscore followed by lowercase, digits, underscore, or hyphen,
// 1 to 32 chars.
//
// UnitSpec.User goes straight into User= in a generated [Service]
// block, rendered unquoted, so this rejects values that could alter
// systemd parsing or inject another directive.
var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func validateSystemUser(name string) error {
	if name == "" {
		return errors.New("user is empty")
	}
	if !usernamePattern.MatchString(name) {
		return fmt.Errorf("user %q must match %s (POSIX username, 1-32 lowercase/digits/underscore/hyphen, leading [a-z_])",
			name, usernamePattern.String())
	}
	return nil
}
