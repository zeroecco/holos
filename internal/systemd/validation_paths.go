package systemd

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateRequiredAbsSafePath(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s path is required", field)
	}
	return validateOptionalAbsSafePath(field, value)
}

func validateOptionalAbsSafePath(field, value string) error {
	if value == "" {
		return nil
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("%s must be absolute, got %q", field, value)
	}
	return rejectUnsafePathChars(field, value)
}

// unsafeExecChars are characters that change systemd's tokenisation
// of an ExecStart/ExecStop line or collide with systemd specifiers.
// systemd(5) parses Exec* with sh(1)-style quoting, expands %x
// specifiers, and treats ; as a conditional command separator. We
// could emit quotes and escape these, but getting that right across
// every systemd version is fragile; refusing to emit a unit with a
// path that would need escaping is a loud, survivable failure.
const unsafeExecChars = " \t\n\r\"'\\`$%;"

func rejectUnsafePathChars(field, value string) error {
	if i := strings.IndexAny(value, unsafeExecChars); i >= 0 {
		return fmt.Errorf(
			"%s %q contains character %q which would break systemd unit parsing; move to a path without whitespace or shell metacharacters",
			field, value, value[i:i+1])
	}
	return nil
}
