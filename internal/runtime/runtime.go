package runtime

import (
	"fmt"

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
	var record *ProjectRecord
	err := m.withProjectLock(project.Name, func() error {
		var err error
		record, err = m.upLocked(project)
		return err
	})
	return record, err
}

func (m *Manager) upLocked(project *compose.Project) (*ProjectRecord, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	record, err := m.loadProjectForUp(project)
	if err != nil {
		return nil, err
	}

	// Pre-provision named volumes before any service starts so that
	// every instance finds its backing file ready. Volumes persist
	// across `down` by design; re-running `up` with an existing
	// state_dir/volumes/<project>/ is a cheap no-op (existing files
	// are left alone, including their contents).
	if err := m.ensureProjectVolumes(project); err != nil {
		return nil, fmt.Errorf("provision volumes: %w", err)
	}

	// Generate (or reuse) the project's `holos exec` keypair and
	// append the public half to every service's authorized_keys.
	//
	// Earlier code wrote the augmented manifest back into
	// project.Services, which quietly mutated the caller's struct.
	// A second Up() on the same *compose.Project (tests, a REPL, a
	// future watch-mode loop) would then find the public key
	// already present and append it a second time, growing
	// authorized_keys unboundedly and changing the spec hash with
	// every call. Keep the augmented manifests in a local map so
	// the caller's Project is read-only from our perspective, as
	// the existing comment always promised.
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
	}

	existingByService := existingServicesByName(record.Services)
	hasDependents := servicesWithDependents(project.ServiceOrder, augmented)

	privKeyPath := privateKeyPath(m.stateDir, project.Name)

	// Service starts are not transactional: each reconcileService
	// spawns QEMU processes that keep running even after this
	// function returns an error. If we bail out of the loop without
	// persisting what got started, `holos ps` shows nothing while
	// real VMs still hold ports, memory, and kernel resources, and
	// `holos down` has no record to target. The only way to
	// recover would be `pkill qemu`, which is both unfriendly and
	// dangerous on shared hosts.
	//
	// Instead we always save whatever made it past reconcileService
	// before returning the loop error. The saved record is a
	// superset: services already started this call, plus carry-over
	// entries for services we never reached so earlier instances
	// aren't silently dropped. A subsequent `holos down <project>`
	// (or a retry `holos up`) then has complete visibility.
	started, upErr := m.reconcileProjectServices(project.Name, project.ServiceOrder, augmented, existingByService, hasDependents, privKeyPath)

	services := started
	if upErr != nil {
		services = carryOverUnreachedServices(started, record.Services)
	} else {
		// Happy path only: tear down services that disappeared from
		// the compose file. On a mid-run failure we skip this so an
		// error in service B does not collaterally stop a healthy
		// service that was simply removed from the file and would
		// have cleaned up on the next Up.
		if err := m.cleanupRemovedServices(project.Name, existingByService, augmented); err != nil {
			upErr = fmt.Errorf("cleanup removed services: %w", err)
		}
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
