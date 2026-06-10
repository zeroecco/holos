package runtime

import (
	"fmt"
	"sort"
)

func existingInstancesByIndex(existing *ServiceRecord) map[int]*InstanceRecord {
	if existing == nil {
		return map[int]*InstanceRecord{}
	}
	instances := make(map[int]*InstanceRecord, len(existing.Instances))
	for i := range existing.Instances {
		inst := &existing.Instances[i]
		refreshInstanceStatus(inst)
		instances[inst.Index] = inst
	}
	return instances
}

func refreshInstanceStatus(inst *InstanceRecord) {
	if inst.PID != 0 && processAlive(inst.PID) {
		inst.Status = InstanceStatusRunning
		return
	}
	inst.Status = InstanceStatusStopped
	inst.PID = 0
}

func attachSortedInstances(svc *ServiceRecord, instances []InstanceRecord) {
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Index < instances[j].Index
	})
	svc.Instances = instances
}

func (m *Manager) stopExcessReplicas(project string, existing *ServiceRecord, replicas int) error {
	if existing == nil {
		return nil
	}
	for i := range existing.Instances {
		inst := &existing.Instances[i]
		if inst.Index >= replicas {
			if err := m.runPreStopCommands(project, *existing, *inst); err != nil {
				return fmt.Errorf("instance %q: %w", inst.Name, err)
			}
			if err := m.stopInstance(*inst); err != nil {
				return fmt.Errorf("instance %q: %w", inst.Name, err)
			}
			markInstanceStopped(inst)
			removeInstanceDir(*inst)
		}
	}
	return nil
}
