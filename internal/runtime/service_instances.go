package runtime

import (
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

func (m *Manager) stopExcessReplicas(existing *ServiceRecord, replicas int) {
	if existing == nil {
		return
	}
	for _, inst := range existing.Instances {
		if inst.Index >= replicas {
			_ = m.stopInstance(inst)
			removeInstanceDir(inst)
		}
	}
}
