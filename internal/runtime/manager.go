package runtime

import (
	"os"
	"path/filepath"
)

const (
	stateDirEnv        = "HOLOS_STATE_DIR"
	rootStateDir       = "/var/lib/holos"
	fallbackStateDir   = ".holos"
	userStateSubdir    = ".local"
	userStateNamespace = "state"
	userStateAppDir    = "holos"
)

// NewManager creates a Manager that stores state under the given directory.
func NewManager(stateDir string) *Manager {
	return &Manager{stateDir: stateDir, lockOptions: DefaultLockOptions()}
}

// DefaultStateDir returns the state directory: HOLOS_STATE_DIR if set,
// /var/lib/holos for root, or ~/.local/state/holos for regular users.
func DefaultStateDir() string {
	if value := os.Getenv(stateDirEnv); value != "" {
		return value
	}

	if os.Geteuid() == 0 {
		return rootStateDir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fallbackStateDir
	}
	return userStateDir(home)
}

func userStateDir(home string) string {
	return filepath.Join(home, userStateSubdir, userStateNamespace, userStateAppDir)
}
