package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	projectsStateSubdir    = "projects"
	instancesStateSubdir   = "instances"
	projectRecordExtension = ".json"
	instanceDirNameFormat  = "%s-%d"
	stateDirPerm           = os.FileMode(0o700)
)

// ensureLayout creates (or tightens) the on-disk state hierarchy.
//
// Mode is 0700 because the tree contains material that should never
// leak to other local users:
//
//   - projects/<name>.json includes generated SSH key paths and host
//     port bindings.
//   - instances/<project>/<inst>/seed/ holds cloud-init user-data,
//     which can carry SSH authorized_keys, write_files (often app
//     secrets), and runcmd entries.
//   - ssh/<project>/ holds the per-project private key holos exec uses.
//
// Existing installations created before this hardening may have 0755
// dirs; chmod them down on every invocation so the migration is silent
// and idempotent.
func (m *Manager) ensureLayout() error {
	for _, dir := range stateLayoutDirs(m.stateDir) {
		if err := os.MkdirAll(dir, stateDirPerm); err != nil {
			return fmt.Errorf("ensure state dir %s: %w", dir, err)
		}
		if err := os.Chmod(dir, stateDirPerm); err != nil {
			return fmt.Errorf("tighten state dir %s: %w", dir, err)
		}
	}
	return nil
}

func stateLayoutDirs(root string) []string {
	return []string{root, projectsDir(root), instancesRoot(root), locksDir(root)}
}

func projectsDir(root string) string {
	return filepath.Join(root, projectsStateSubdir)
}

func instancesRoot(root string) string {
	return filepath.Join(root, instancesStateSubdir)
}

func projectFile(root, name string) string {
	return filepath.Join(projectsDir(root), name+projectRecordExtension)
}

func projectRecordsGlob(root string) string {
	return filepath.Join(projectsDir(root), "*"+projectRecordExtension)
}

func projectInstanceDir(root, project, service string, index int) string {
	return filepath.Join(instancesRoot(root), project, instanceDirName(service, index))
}

func instanceDirName(service string, index int) string {
	return fmt.Sprintf(instanceDirNameFormat, service, index)
}
