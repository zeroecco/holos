package runtime

import (
	"errors"
	"os"
	"time"

	"github.com/zeroecco/holos/internal/compose"
)

func (m *Manager) loadProjectForUp(project *compose.Project) (*ProjectRecord, error) {
	record, err := m.loadProject(project.Name)
	if projectLoadErrorBlocksUp(err) {
		return nil, err
	}

	if shouldResetProjectState(record, project.SpecHash) {
		if err := m.tearDownProject(record); err != nil {
			return nil, err
		}
		record = nil
	}

	if record == nil {
		record = &ProjectRecord{Name: project.Name}
	}
	return record, nil
}

func shouldResetProjectState(record *ProjectRecord, nextSpecHash string) bool {
	return record != nil && record.SpecHash != "" && record.SpecHash != nextSpecHash
}

func projectLoadErrorBlocksUp(err error) bool {
	return err != nil && !errors.Is(err, os.ErrNotExist)
}

func (m *Manager) tearDownProject(record *ProjectRecord) error {
	for i := len(record.Services) - 1; i >= 0; i-- {
		m.removeInstances(record.Services[i].Instances)
	}
	return nil
}

func (m *Manager) refreshProject(record *ProjectRecord) {
	for i := range record.Services {
		for j := range record.Services[i].Instances {
			refreshInstanceStatus(&record.Services[i].Instances[j])
		}
	}
}

func (m *Manager) saveUpdatedProject(record *ProjectRecord) error {
	record.UpdatedAt = time.Now().UTC()
	return m.saveProject(record)
}
