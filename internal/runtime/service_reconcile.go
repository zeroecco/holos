package runtime

import (
	"os"

	"github.com/zeroecco/holos/internal/config"
)

// carryOverUnreachedServices retains prior services outside a scoped start or
// unreached after a launch failure. Fresh records take precedence; surviving
// VMs must remain tracked even if absent from the requested service set.
func carryOverUnreachedServices(started, prior []ServiceRecord) []ServiceRecord {
	seen := make(map[string]struct{}, len(started))
	for _, s := range started {
		seen[s.Name] = struct{}{}
	}
	out := append([]ServiceRecord(nil), started...)
	for _, p := range prior {
		if _, touched := seen[p.Name]; touched {
			continue
		}
		out = append(out, p)
	}
	return out
}

// augmentServicesWithExecKey copies the map and authorized-key slices before
// adding the generated key, keeping the caller's manifests unchanged.
func augmentServicesWithExecKey(services map[string]config.Manifest, pubKey string) map[string]config.Manifest {
	out := make(map[string]config.Manifest, len(services))
	for name, manifest := range services {
		manifest.CloudInit.SSHAuthorizedKeys = append(
			append([]string(nil), manifest.CloudInit.SSHAuthorizedKeys...),
			pubKey,
		)
		out[name] = manifest
	}
	return out
}

// reconcileService ensures a service has the desired number of running instances.
func (m *Manager) reconcileService(project string, manifest config.Manifest, existing *ServiceRecord) (*ServiceRecord, error) {
	svc := serviceRecordForManifest(manifest)

	existingInstances := existingInstancesByIndex(existing)

	// Build up instances incrementally. If any replica start fails
	// partway through we return a partial ServiceRecord alongside the
	// error so the caller can persist replicas that did start.
	instances := make([]InstanceRecord, 0, manifest.Replicas)
	sortAndAttach := func() {
		attachSortedInstances(svc, instances)
	}
	for index := 0; index < manifest.Replicas; index++ {
		inst, err := m.reconcileServiceInstance(project, manifest, index, existingInstances[index])
		if err != nil {
			sortAndAttach()
			return svc, err
		}
		instances = append(instances, inst)
	}

	if err := m.stopExcessReplicas(project, existing, manifest.Replicas); err != nil {
		sortAndAttach()
		return svc, err
	}

	sortAndAttach()
	return svc, nil
}

func (m *Manager) reconcileServiceInstance(project string, manifest config.Manifest, index int, existing *InstanceRecord) (InstanceRecord, error) {
	if shouldReuseInstance(existing) {
		return *existing, nil
	}
	if shouldRestartInstance(existing) {
		return m.restartInstance(project, manifest, *existing)
	}
	return m.startInstance(project, manifest, index)
}

func serviceRecordForManifest(manifest config.Manifest) *ServiceRecord {
	return &ServiceRecord{
		Name:            manifest.Name,
		DesiredReplicas: manifest.Replicas,
		LoginUser:       manifest.CloudInit.User,
		PreStopCommands: append([]string(nil), manifest.PreStopCommands...),
	}
}

func shouldReuseInstance(inst *InstanceRecord) bool {
	return inst != nil && inst.Status == InstanceStatusRunning
}

func shouldRestartInstance(inst *InstanceRecord) bool {
	if inst == nil || inst.WorkDir == "" {
		return false
	}
	info, err := os.Stat(inst.WorkDir)
	return err == nil && info.IsDir()
}
