package main

import (
	"errors"
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/runtime"
)

type instanceTarget struct {
	Inst        runtime.InstanceRecord
	ProjectName string
	LoginUser   string
	CmdArgs     []string
}

func resolveInstanceTarget(manager *runtime.Manager, filePath, stateDir string, positional []string) (instanceTarget, error) {
	if len(positional) == 0 {
		return instanceTarget{}, errors.New("missing project or instance name")
	}
	if filePath == "" {
		if err := compose.ValidateName(positional[0]); err != nil {
			return instanceTarget{}, fmt.Errorf("invalid project name: %w", err)
		}
		if record, ok := lookupProjectRecord(manager, positional[0]); ok {
			return resolveProjectInstanceTarget(record, stateDir, positional)
		}
	}

	file, _, err := loadComposeFile(filePath)
	if err != nil {
		return instanceTarget{}, err
	}
	record, err := manager.ProjectStatus(file.Name)
	if err != nil {
		return instanceTarget{}, err
	}
	inst, service, err := resolveRecordInstance(record, positional[0])
	if err != nil {
		return instanceTarget{}, err
	}
	return instanceTarget{
		Inst:        inst,
		ProjectName: record.Name,
		LoginUser:   serviceLoginUser(service, record, stateDir),
		CmdArgs:     positional[1:],
	}, nil
}

// A project target may name a guest or, for a single-guest project, go straight
// to the command. Keep that argument handling separate from compose lookup.
func resolveProjectInstanceTarget(record *runtime.ProjectRecord, stateDir string, positional []string) (instanceTarget, error) {
	target := instanceTarget{ProjectName: record.Name}
	if len(positional) >= 2 {
		if inst, svc, err := resolveRecordInstance(record, positional[1]); err == nil {
			target.Inst = inst
			target.LoginUser = serviceLoginUser(svc, record, stateDir)
			target.CmdArgs = positional[2:]
			return target, nil
		}
	}
	inst, svc, ok := soleInstance(record)
	if !ok {
		return instanceTarget{}, fmt.Errorf(
			"project %q has multiple instances%s; specify one (available: %s)",
			record.Name, missingInstanceDetail(positional), instanceList(record))
	}
	target.Inst = inst
	target.LoginUser = serviceLoginUser(svc, record, stateDir)
	target.CmdArgs = positional[1:]
	return target, nil
}

// Exact instance names take precedence. Service shorthand is safe only when
// there is one replica; never silently choose a machine for an exec command.
func resolveRecordInstance(record *runtime.ProjectRecord, name string) (runtime.InstanceRecord, runtime.ServiceRecord, error) {
	if inst, service, ok := findInstanceInRecord(record, name); ok {
		return inst, service, nil
	}
	for _, service := range record.Services {
		if service.Name != name {
			continue
		}
		if len(service.Instances) == 1 {
			return service.Instances[0], service, nil
		}
		available := instanceList(&runtime.ProjectRecord{Services: []runtime.ServiceRecord{service}})
		return runtime.InstanceRecord{}, runtime.ServiceRecord{}, fmt.Errorf(
			"service %q has %d instances; specify an instance (available: %s)",
			name, len(service.Instances), available)
	}
	return runtime.InstanceRecord{}, runtime.ServiceRecord{}, fmt.Errorf("no service or instance %q in project %q (available: %s)", name, record.Name, instanceList(record))
}

func missingInstanceDetail(positional []string) string {
	if len(positional) < 2 {
		return ""
	}
	return fmt.Sprintf(" (no instance %q)", positional[1])
}

func serviceLoginUser(svc runtime.ServiceRecord, record *runtime.ProjectRecord, stateDir string) string {
	if svc.LoginUser != "" {
		return svc.LoginUser
	}
	if record == nil {
		return ""
	}
	if user := projectRecordLoginUser(record); user != "" {
		return user
	}
	return lookupLoginUser(stateDir, record.Name)
}

func projectRecordLoginUser(record *runtime.ProjectRecord) string {
	for _, svc := range record.Services {
		if svc.LoginUser != "" {
			return svc.LoginUser
		}
	}
	return ""
}

func lookupLoginUser(stateDir, projectName string) string {
	composePath := runComposeFilePath(stateDir, projectName)
	file, err := compose.Load(composePath)
	if err != nil {
		return ""
	}
	project, err := file.Resolve(runComposeProjectDir(stateDir, projectName), stateDir)
	if err != nil {
		return ""
	}
	return composeProjectLoginUser(project)
}

func composeProjectLoginUser(project *compose.Project) string {
	for _, svc := range project.Services {
		if svc.CloudInit.User != "" {
			return svc.CloudInit.User
		}
	}
	return ""
}
