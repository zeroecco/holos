package main

import (
	"strings"

	"github.com/zeroecco/holos/internal/runtime"
)

const instanceListSeparator = ", "

func lookupProjectRecord(manager *runtime.Manager, name string) (*runtime.ProjectRecord, bool) {
	if name == "" {
		return nil, false
	}
	record, err := manager.ProjectStatus(name)
	if err != nil {
		return nil, false
	}
	return record, true
}

func findInstanceInRecord(record *runtime.ProjectRecord, instanceName string) (runtime.InstanceRecord, runtime.ServiceRecord, bool) {
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Name == instanceName {
				return inst, svc, true
			}
		}
	}
	return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
}

func soleInstance(record *runtime.ProjectRecord) (runtime.InstanceRecord, runtime.ServiceRecord, bool) {
	var (
		hitInst runtime.InstanceRecord
		hitSvc  runtime.ServiceRecord
		count   int
	)
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			count++
			if count > 1 {
				return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
			}
			hitInst = inst
			hitSvc = svc
		}
	}
	if count == 1 {
		return hitInst, hitSvc, true
	}
	return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
}

func instanceList(record *runtime.ProjectRecord) string {
	instances := instancesInRecord(record)
	names := make([]string, 0, len(instances))
	for _, inst := range instances {
		names = append(names, inst.Name)
	}
	return strings.Join(names, instanceListSeparator)
}

func logTargets(record *runtime.ProjectRecord, filter string) []runtime.InstanceRecord {
	if filter != "" {
		return resolveLogTargets(record, filter)
	}
	return instancesInRecord(record)
}

func instancesInRecord(record *runtime.ProjectRecord) []runtime.InstanceRecord {
	var instances []runtime.InstanceRecord
	for _, svc := range record.Services {
		instances = append(instances, svc.Instances...)
	}
	return instances
}

func resolveLogTargets(record *runtime.ProjectRecord, name string) []runtime.InstanceRecord {
	if instances, ok := instancesForServiceName(record, name); ok {
		return instances
	}
	if inst, ok := instanceForName(record, name); ok {
		return []runtime.InstanceRecord{inst}
	}
	return nil
}

func instancesForServiceName(record *runtime.ProjectRecord, name string) ([]runtime.InstanceRecord, bool) {
	for _, svc := range record.Services {
		if svc.Name == name {
			return svc.Instances, true
		}
	}
	return nil, false
}

func instanceForName(record *runtime.ProjectRecord, name string) (runtime.InstanceRecord, bool) {
	inst, _, ok := findInstanceInRecord(record, name)
	return inst, ok
}
