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
	if err := m.ensureLayout(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project record: %w", err)
	}
	// 0600: the record embeds host port bindings and per-project ssh
	// key paths; no reason any other local user needs to read it.
	return os.WriteFile(projectFile(m.stateDir, record.Name), payload, projectRecordPerm)
}
