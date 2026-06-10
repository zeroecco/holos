package main

import (
	"errors"
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
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
			tgt := instanceTarget{ProjectName: record.Name}
			if len(positional) >= 2 {
				if inst, svc, ok := findInstanceInRecord(record, positional[1]); ok {
					tgt.Inst = inst
					tgt.LoginUser = serviceLoginUser(svc, record, stateDir)
					tgt.CmdArgs = positional[2:]
					return tgt, nil
				}
			}
			inst, svc, ok := soleInstance(record)
			if !ok {
				return instanceTarget{}, fmt.Errorf(
					"project %q has multiple instances%s; specify one (available: %s)",
					record.Name, missingInstanceDetail(positional), instanceList(record))
			}
			tgt.Inst = inst
			tgt.LoginUser = serviceLoginUser(svc, record, stateDir)
			tgt.CmdArgs = positional[1:]
			return tgt, nil
		}
	}

	project, err := loadProject(filePath, stateDir)
	if err != nil {
		return instanceTarget{}, err
	}
	inst, svcName, err := manager.FindInstance(project.Name, positional[0])
	if err != nil {
		return instanceTarget{}, err
	}
	tgt := instanceTarget{
		Inst:        inst,
		ProjectName: project.Name,
		CmdArgs:     positional[1:],
	}
	if svc, ok := project.Services[svcName]; serviceHasLoginUser(svc, ok) {
		tgt.LoginUser = svc.CloudInit.User
	}
	return tgt, nil
}

func serviceHasLoginUser(svc config.Manifest, found bool) bool {
	return found && svc.CloudInit.User != ""
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
	if record != nil {
		if user := projectRecordLoginUser(record); user != "" {
			return user
		}
		return lookupLoginUser(stateDir, record.Name)
	}
	return ""
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
