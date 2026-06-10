package systemd

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	systemUnitDir     = "/etc/systemd/system"
	xdgConfigHomeEnv  = "XDG_CONFIG_HOME"
	xdgConfigHomeDir  = ".config"
	xdgUserUnitSubdir = "systemd/user"
)

// UnitPath returns where a unit for the given project/scope lives,
// without touching the filesystem. Callers can use this to present a
// dry-run path to the user before committing.
func UnitPath(scope Scope, project string) (string, error) {
	if err := validateProjectName(project); err != nil {
		return "", err
	}
	name := unitFilename(project)
	switch scope {
	case ScopeUser:
		dir, err := userUnitDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, name), nil
	case ScopeSystem:
		return filepath.Join(systemUnitDir, name), nil
	default:
		return "", fmt.Errorf("unknown scope %q", scope)
	}
}

func unitFilename(project string) string {
	return fmt.Sprintf("holos-%s.service", project)
}

func userUnitDir() (string, error) {
	// XDG_CONFIG_HOME is the systemd-documented location. If unset,
	// ~/.config/systemd/user is the conventional fallback.
	if cfg := os.Getenv(xdgConfigHomeEnv); cfg != "" {
		return filepath.Join(cfg, filepath.FromSlash(xdgUserUnitSubdir)), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, xdgConfigHomeDir, filepath.FromSlash(xdgUserUnitSubdir)), nil
}
