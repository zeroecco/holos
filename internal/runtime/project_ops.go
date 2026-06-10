package runtime

import (
	"fmt"
	"os"
)

// Down stops and removes all resources for a project.
func (m *Manager) Down(projectName string) error {
	return m.withProjectLock(projectName, func() error {
		record, err := m.loadProject(projectName)
		if err != nil {
			return err
		}

		if err := m.tearDownProject(record); err != nil {
			return err
		}

		return os.Remove(projectFile(m.stateDir, projectName))
	})
}

// StopProject stops all services without removing state.
func (m *Manager) StopProject(projectName string) (*ProjectRecord, error) {
	var record *ProjectRecord
	err := m.withProjectLock(projectName, func() error {
		var err error
		record, err = m.stopProjectLocked(projectName)
		return err
	})
	return record, err
}

func (m *Manager) stopProjectLocked(projectName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	for i := range record.Services {
		if err := m.stopServiceInstances(projectName, &record.Services[i]); err != nil {
			return nil, fmt.Errorf("service %q: %w", record.Services[i].Name, err)
		}
	}

	if err := m.saveUpdatedProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// StopService stops a single service within a project.
func (m *Manager) StopService(projectName, serviceName string) (*ProjectRecord, error) {
	var record *ProjectRecord
	err := m.withProjectLock(projectName, func() error {
		var err error
		record, err = m.stopServiceLocked(projectName, serviceName)
		return err
	})
	return record, err
}

func (m *Manager) stopServiceLocked(projectName, serviceName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	service, ok := findServiceRecord(record, serviceName)
	if !ok {
		return nil, serviceNotFoundError(projectName, serviceName)
	}
	if err := m.stopServiceInstances(projectName, service); err != nil {
		return nil, fmt.Errorf("service %q: %w", service.Name, err)
	}

	if err := m.saveUpdatedProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

func serviceNotFoundError(projectName, serviceName string) error {
	return fmt.Errorf("service %q not found in project %q", serviceName, projectName)
}

func findServiceRecord(record *ProjectRecord, serviceName string) (*ServiceRecord, bool) {
	for i := range record.Services {
		if record.Services[i].Name == serviceName {
			return &record.Services[i], true
		}
	}
	return nil, false
}
