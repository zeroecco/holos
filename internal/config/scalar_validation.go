package config

import "fmt"

// ValidateUserName checks the guest account name holos asks cloud-init to
// create. Keep this deliberately aligned with the systemd User= validation:
// lowercase POSIX names are portable across the distro images holos targets and
// cannot inject shell, YAML, or unit-file syntax through later command paths.
func ValidateUserName(name string) error {
	if emptyScalar(name) {
		return fmt.Errorf("user is empty")
	}
	if !userNamePattern.MatchString(name) {
		return fmt.Errorf("user %q must match %s (POSIX username, 1-32 lowercase/digits/underscore/hyphen, leading [a-z_])",
			name, userNamePattern.String())
	}
	return nil
}

// ValidatePCIAddress checks a canonical PCI BDF address:
// domain:bus:slot.function, with the function constrained to 0-7.
func ValidatePCIAddress(addr string) error {
	if emptyScalar(addr) {
		return fmt.Errorf("address is empty")
	}
	if !pciAddressPattern.MatchString(addr) {
		return fmt.Errorf("must match 0000:01:00.0")
	}
	return nil
}

func emptyScalar(value string) bool {
	return value == ""
}
