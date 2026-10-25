package runtime

import (
	"errors"
	"os"
	"time"

	"github.com/zeroecco/holos/internal/compose"
)

func (m *Manager) loadProjectForUp(project *compose.Project) (*ProjectRecord, error) {
	record, err := m.loadProject(project.Name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if record == nil {
		record = &ProjectRecord{Name: project.Name}
	}
	return record, nil
}

func (m *Manager) tearDownProject(record *ProjectRecord) error {
	for i := len(record.Services) - 1; i >= 0; i-- {
		if err := m.stopServiceInstances(record.Name, &record.Services[i]); err != nil {
			return err
		}
		m.removeInstanceDirs(record.Services[i].Instances)
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
