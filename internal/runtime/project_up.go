package runtime

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func existingServicesByName(services []ServiceRecord) map[string]*ServiceRecord {
	existingByService := make(map[string]*ServiceRecord)
	for i := range services {
		existingByService[services[i].Name] = &services[i]
	}
	return existingByService
}

// servicesWithDependents is the set of services whose health we must confirm
// before starting their consumers. Services without dependents still have their
// healthcheck declared for `ps` visibility, but we don't block on them, matching
// docker's convention that healthchecks are only a gating tool.
func servicesWithDependents(order []string, services map[string]config.Manifest) map[string]bool {
	hasDependents := make(map[string]bool)
	for _, svc := range order {
		for _, dep := range services[svc].DependsOn {
			hasDependents[dep] = true
		}
	}
	return hasDependents
}

func (m *Manager) reconcileProjectServices(projectName string, order []string, services map[string]config.Manifest, existingByService map[string]*ServiceRecord, hasDependents map[string]bool, privKeyPath string) ([]ServiceRecord, error) {
	var started []ServiceRecord
	for _, svcName := range order {
		manifest := services[svcName]
		existing := existingByService[svcName]

		svcRecord, err := m.reconcileService(projectName, manifest, existing)
		if err != nil {
			// reconcileService returns a partial ServiceRecord on
			// error so we can still persist the replicas that did
			// start. Append it even on failure; the alternative is
			// an orphaned QEMU process with no tracking record.
			if serviceRecordHasInstances(svcRecord) {
				started = append(started, *svcRecord)
			}
			return started, fmt.Errorf("service %q: %w", svcName, err)
		}
		started = append(started, *svcRecord)

		if shouldWaitForServiceHealth(manifest, hasDependents[svcName]) {
			if err := m.waitForServiceHealthy(svcRecord, manifest, privKeyPath); err != nil {
				return started, fmt.Errorf("service %q unhealthy: %w", svcName, err)
			}
		}
	}
	return started, nil
}

func serviceRecordHasInstances(record *ServiceRecord) bool {
	return record != nil && len(record.Instances) > 0
}

func shouldWaitForServiceHealth(manifest config.Manifest, hasDependents bool) bool {
	return manifest.Healthcheck != nil && hasDependents
}

func (m *Manager) cleanupRemovedServices(existingByService map[string]*ServiceRecord, desired map[string]config.Manifest) {
	for name, existing := range existingByService {
		if _, ok := desired[name]; ok {
			continue
		}
		m.removeInstances(existing.Instances)
	}
}
