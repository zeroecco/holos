package systemd

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	systemdUnitDirPerm  = os.FileMode(0o755)
	systemdUnitFilePerm = os.FileMode(0o644)
)

// Install writes the unit to disk (creating parent dirs as needed)
// and, unless Dry is set on the returned Result, daemon-reload + enable
// it so the service survives reboot. Missing systemctl is tolerated
// for user scope (e.g. running in a minimal container); the unit file
// is still produced so the operator can install it by hand.
func Install(spec UnitSpec, enable bool) (Result, error) {
	path, content, err := Render(spec)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), systemdUnitDirPerm); err != nil {
		return Result{}, fmt.Errorf("create unit dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), systemdUnitFilePerm); err != nil {
		return Result{}, fmt.Errorf("write unit: %w", err)
	}

	res := Result{UnitPath: path, Scope: spec.Scope}

	if !systemctlAvailable() {
		res.SystemctlMissing = true
		return res, nil
	}

	if err := systemctlReload(spec.Scope); err != nil {
		// A daemon-reload failure is non-fatal: the unit is on disk
		// and a subsequent manual reload will pick it up. We surface
		// the error on the Result so the CLI can warn.
		res.Warnings = append(res.Warnings, systemctlWarning("daemon-reload", err))
	}

	if enable {
		if err := systemctlEnable(spec.Scope, path, true); err != nil {
			res.Warnings = append(res.Warnings, systemctlWarning("enable", err))
		} else {
			res.Enabled = true
		}
	}
	return res, nil
}

// Uninstall disables the unit (best effort) and removes its file. A
// missing file is not an error; it makes the command idempotent.
func Uninstall(scope Scope, project string) (Result, error) {
	path, err := UnitPath(scope, project)
	if err != nil {
		return Result{}, err
	}
	res := Result{UnitPath: path, Scope: scope}

	if systemctlAvailable() {
		if err := systemctlDisable(scope, path); err != nil {
			res.Warnings = append(res.Warnings, systemctlWarning("disable", err))
		}
	} else {
		res.SystemctlMissing = true
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return res, fmt.Errorf("remove unit: %w", err)
	}

	if systemctlAvailable() {
		_ = systemctlReload(scope)
	}
	return res, nil
}

func systemctlWarning(operation string, err error) string {
	return fmt.Sprintf("%s: %v", operation, err)
}

// Result describes what Install/Uninstall did, for CLI reporting.
type Result struct {
	UnitPath         string
	Scope            Scope
	Enabled          bool
	SystemctlMissing bool
	Warnings         []string
}
