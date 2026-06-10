package systemd

import (
	"fmt"
)

func (s UnitSpec) validate() error {
	// The DNS-label check already rejects whitespace, slashes, and
	// every other character that systemd treats specially, so it
	// subsumes the earlier "no slash or whitespace" rule.
	if err := validateProjectName(s.Project); err != nil {
		return err
	}
	if err := validateRequiredAbsSafePath("compose file", s.ComposeFile); err != nil {
		return err
	}
	if err := validateRequiredAbsSafePath("holos binary", s.HolosBinary); err != nil {
		return err
	}
	if err := validateOptionalAbsSafePath("state dir", s.StateDir); err != nil {
		return err
	}
	switch s.Scope {
	case ScopeUser, ScopeSystem:
	default:
		return fmt.Errorf("unknown scope %q", s.Scope)
	}
	if s.User != "" {
		// Only system-scope units honor User=, but validate for any
		// scope: a caller that sets .User on a user-scope unit is
		// already wrong, and if we change scope handling later we
		// don't want a previously-accepted bad value sneaking in.
		if err := validateSystemUser(s.User); err != nil {
			return err
		}
	}
	return nil
}
