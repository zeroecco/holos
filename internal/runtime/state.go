package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zeroecco/holos/internal/compose"
)

const projectRecordPerm = os.FileMode(0o600)

// ListProjects returns all known projects.
func (m *Manager) ListProjects() ([]*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(projectRecordsGlob(m.stateDir))
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	projects := make([]*ProjectRecord, 0, len(matches))
	for _, match := range matches {
		payload, err := os.ReadFile(match)
		if err != nil {
			return nil, fmt.Errorf("read project %s: %w", match, err)
		}
		var record ProjectRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, fmt.Errorf("decode project %s: %w", match, err)
		}
		m.refreshProject(&record)
		projects = append(projects, &record)
	}

	sortProjectRecords(projects)
	return projects, nil
}

func sortProjectRecords(projects []*ProjectRecord) {
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})
}

func (m *Manager) loadProject(name string) (*ProjectRecord, error) {
	// Defense in depth: every lookup that can accept a user-provided
	// project name (Down, ProjectStatus, FindInstance, ...) funnels
	// through here, so validating once keeps path traversal attempts
	// from reaching os.ReadFile/os.Remove regardless of which CLI
	// command the caller came from.
	if err := compose.ValidateName(name); err != nil {
		return nil, fmt.Errorf("invalid project name: %w", err)
	}
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}
	path := projectFile(m.stateDir, name)
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record ProjectRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, fmt.Errorf("decode project record: %w", err)
	}
	return &record, nil
}

func (m *Manager) saveProject(record *ProjectRecord) error {
	if record == nil {
		return fmt.Errorf("save project: record is nil")
	}
	if err := compose.ValidateName(record.Name); err != nil {
		return fmt.Errorf("save project: invalid project name: %w", err)
	}
	if err := m.ensureLayout(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project record: %w", err)
	}
	// Project records are the recovery source after the CLI exits. Write a
	// complete replacement in the same directory, sync it, and only then rename
	// it over the live record. A crash or ENOSPC during a direct os.WriteFile
	// could otherwise truncate the only copy and make the whole project
	// impossible to inspect, stop, or remove cleanly.
	return writeProjectRecord(projectFile(m.stateDir, record.Name), payload)
}

func writeProjectRecord(path string, payload []byte) (retErr error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".project-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary project record: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmp != nil {
			if closeErr := tmp.Close(); retErr == nil && closeErr != nil {
				retErr = fmt.Errorf("close temporary project record: %w", closeErr)
			}
		}
		if removeErr := os.Remove(tmpPath); retErr == nil && removeErr != nil && !os.IsNotExist(removeErr) {
			retErr = fmt.Errorf("remove temporary project record: %w", removeErr)
		}
	}()

	// 0600: the record embeds host port bindings and per-project SSH key
	// paths; no other local user needs to read it. Chmod explicitly rather than
	// relying on the process umask so an unusually permissive environment
	// cannot weaken the state file.
	if err := tmp.Chmod(projectRecordPerm); err != nil {
		return fmt.Errorf("set temporary project record permissions: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		return fmt.Errorf("write temporary project record: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary project record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary project record: %w", err)
	}
	tmp = nil
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace project record: %w", err)
	}

	// Persist the directory entry as well as the file contents. Linux and
	// macOS both support syncing a directory; if a filesystem rejects it, the
	// caller gets an honest durability error instead of a false success.
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open project directory for sync: %w", err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync project directory: %w", err)
	}
	return nil
}
