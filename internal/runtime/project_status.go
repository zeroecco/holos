package runtime

import (
	"fmt"
)

// ProjectStatus returns the current state of a project, refreshing PID liveness.
func (m *Manager) ProjectStatus(projectName string) (*ProjectRecord, error) {
	var record *ProjectRecord
	err := m.withProjectLock(projectName, func() error {
		var err error
		record, err = m.projectStatusLocked(projectName)
		return err
	})
	return record, err
}

func (m *Manager) projectStatusLocked(projectName string) (*ProjectRecord, error) {
	record, err := m.loadProject(projectName)
	if err != nil {
		return nil, err
	}

	m.refreshProject(record)
	if err := m.saveUpdatedProject(record); err != nil {
		return nil, err
	}
	return record, nil
}

// FindInstance locates an instance within a project by its short name
// (e.g. "web-0"). Returns the instance along with the service it
// belongs to so callers can surface useful errors. The returned record
// reflects the on-disk state of the instance after a PID liveness
// refresh.
func (m *Manager) FindInstance(projectName, instanceName string) (InstanceRecord, string, error) {
	record, err := m.ProjectStatus(projectName)
	if err != nil {
		return InstanceRecord{}, "", err
	}
	if inst, serviceName, ok := findInstanceRecord(record, instanceName); ok {
		return inst, serviceName, nil
	}
	return InstanceRecord{}, "", instanceNotFoundError(projectName, instanceName)
}

func instanceNotFoundError(projectName, instanceName string) error {
	return fmt.Errorf("instance %q not found in project %q", instanceName, projectName)
}

func findInstanceRecord(record *ProjectRecord, instanceName string) (InstanceRecord, string, bool) {
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Name == instanceName {
				return inst, svc.Name, true
			}
		}
	}
	return InstanceRecord{}, "", false
}

// ProjectSSHKeyPath returns the path to a project's `holos exec`
// private key, creating the keypair if it doesn't already exist. This
// is the entry point used by the exec command, and must not depend on
// any prior Up having run (e.g. for `holos exec` from a fresh shell).
func (m *Manager) ProjectSSHKeyPath(projectName string) (string, error) {
	if err := m.ensureLayout(); err != nil {
		return "", err
	}
	privPath, _, err := ensureProjectSSHKey(m.stateDir, projectName)
	return privPath, err
}
