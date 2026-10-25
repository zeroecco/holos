package runtime

import (
	"fmt"
	"sort"

	"github.com/zeroecco/holos/internal/compose"
)

// Manager coordinates project lifecycle and state persistence.
type Manager struct {
	stateDir    string
	lockOptions LockOptions
}

// Up brings a compose project to the desired state, starting services
// in topological order.
func (m *Manager) Up(project *compose.Project) (*ProjectRecord, error) {
	return m.UpWithOptions(project, UpOptions{})
}

// UpWithOptions previews or applies a service-level update under one lock.
func (m *Manager) UpWithOptions(project *compose.Project, options UpOptions) (*ProjectRecord, error) {
	var record *ProjectRecord
	err := m.withProjectLock(project.Name, func() error {
		var err error
		record, err = m.upLocked(project, options)
		return err
	})
	return record, err
}

func (m *Manager) upLocked(project *compose.Project, options UpOptions) (*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	record, err := m.loadProjectForUp(project)
	if err != nil {
		return nil, err
	}

	changes := projectChanges(project, record)
	if options.PreserveOtherServices {
		selected := changes[:0]
		for _, change := range changes {
			if change.Action != ChangeRemove {
				selected = append(selected, change)
			}
		}
		changes = selected
	}
	if options.Report != nil {
		options.Report(changes)
	}
	if options.DryRun {
		return record, nil
	}
	if err := m.prepareUpdates(record, changes, options.Recreate); err != nil {
		return record, err
	}

	// Provision backing files before launch; existing named volumes remain intact.
	if err := m.ensureProjectVolumes(project); err != nil {
		return nil, fmt.Errorf("provision volumes: %w", err)
	}

	// Add the generated SSH key to copies so repeated calls do not mutate input.
	_, pubKey, err := ensureProjectSSHKey(m.stateDir, project.Name)
	if err != nil {
		return nil, fmt.Errorf("ensure exec ssh key: %w", err)
	}
	augmented := augmentServicesWithExecKey(project.Services, pubKey)

	record.SpecHash = project.SpecHash
	record.Volumes = volumeRecordsForProject(project)
	record.Network = NetworkState{
		MulticastGroup: project.Network.MulticastGroup,
		MulticastPort:  project.Network.MulticastPort,
		Subnet:         project.Network.Subnet,
		Hosts:          project.Network.Hosts,
		Segments:       networkSegmentRecords(project.Network),
	}

	existingByService := existingServicesByName(record.Services)
	hasDependents := servicesWithDependents(project.ServiceOrder, augmented)

	privKeyPath := privateKeyPath(m.stateDir, project.Name)

	// Launches are not transactional. Persist newly started and unreached
	// services on failure so every surviving VM remains manageable.
	services, upErr := m.reconcileProjectServices(project.Name, project.ServiceOrder, augmented, existingByService, hasDependents, privKeyPath)

	for i := range services {
		services[i].SpecHash = serviceFingerprint(project.Services[services[i].Name])
	}

	if upErr != nil || options.PreserveOtherServices {
		services = carryOverUnreachedServices(services, record.Services)
	}

	record.Services = services

	if saveErr := m.saveUpdatedProject(record); saveErr != nil {
		if upErr != nil {
			return nil, fmt.Errorf("%w (also failed to persist partial state: %v)", upErr, saveErr)
		}
		return nil, saveErr
	}
	if upErr != nil {
		return record, upErr
	}
	return record, nil
}

func networkSegmentRecords(network compose.NetworkPlan) []NetworkSegmentState {
	segments := make([]NetworkSegmentState, 0, len(network.Segments))
	names := make([]string, 0, len(network.Segments))
	for name := range network.Segments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		segment := network.Segments[name]
		segments = append(segments, NetworkSegmentState{
			Name:           name,
			MulticastGroup: segment.MulticastGroup,
			MulticastPort:  segment.MulticastPort,
			Subnet:         segment.Subnet,
			Hosts:          segment.Hosts,
		})
	}
	return segments
}
