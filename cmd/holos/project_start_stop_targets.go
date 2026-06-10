package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/runtime"
)

func resolveStartTarget(manager *runtime.Manager, filePath, stateDir string, args []string) (string, string, bool, error) {
	if !canResolveProjectArg(filePath, args) {
		return filePath, "", false, nil
	}

	candidate := args[0]
	if err := compose.ValidateName(candidate); err != nil {
		return "", "", false, fmt.Errorf("invalid project name: %w", err)
	}
	if _, ok := lookupProjectRecord(manager, candidate); !ok {
		return filePath, "", false, nil
	}

	return runComposeFilePath(stateDir, candidate), serviceArgAfterProject(args), true, nil
}

func limitProjectToService(project *compose.Project, svcName string) error {
	if svcName == "" {
		return nil
	}
	if _, ok := project.Services[svcName]; !ok {
		return projectServiceNotFoundError(project.Name, svcName)
	}
	for name := range project.Services {
		if name != svcName {
			delete(project.Services, name)
		}
	}
	project.ServiceOrder = []string{svcName}
	return nil
}

func resolveStopTarget(manager *runtime.Manager, filePath, stateDir string, args []string) (string, string, error) {
	if canResolveProjectArg(filePath, args) {
		candidate := args[0]
		if err := compose.ValidateName(candidate); err != nil {
			return "", "", fmt.Errorf("invalid project name: %w", err)
		}
		if _, ok := lookupProjectRecord(manager, candidate); ok {
			return candidate, serviceArgAfterProject(args), nil
		}
	}

	project, err := loadProject(filePath, stateDir)
	if err != nil {
		return "", "", err
	}
	svcName := ""
	if len(args) > 0 {
		svcName = args[0]
	}
	return project.Name, svcName, nil
}

func canResolveProjectArg(filePath string, args []string) bool {
	return filePath == "" && len(args) > 0
}

func projectServiceNotFoundError(projectName, serviceName string) error {
	return fmt.Errorf("service %q not found in project %q", serviceName, projectName)
}

func serviceArgAfterProject(args []string) string {
	if len(args) <= 1 {
		return ""
	}
	return args[1]
}
