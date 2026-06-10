package main

import (
	"github.com/zeroecco/holos/internal/runtime"
)

func runStart(args []string) error {
	flags := newFlagSet("start")
	projectFlags := addProjectFlags(flags, "")
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	projectFile, svcName, resolvedProjectArg, err := resolveStartTarget(manager, *projectFlags.filePath, *projectFlags.stateDir, flags.Args())
	if err != nil {
		return err
	}

	project, err := loadProject(projectFile, *projectFlags.stateDir)
	if err != nil {
		return err
	}

	svcName = startServiceName(svcName, resolvedProjectArg, flags.Args())
	if err := limitProjectToService(project, svcName); err != nil {
		return err
	}

	applyLockFlags(manager, lock)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func startServiceName(resolvedService string, resolvedProjectArg bool, args []string) string {
	if resolvedService == "" && !resolvedProjectArg && len(args) > 0 {
		return args[0]
	}
	return resolvedService
}

func runStop(args []string) error {
	flags := newFlagSet("stop")
	projectFlags := addProjectFlags(flags, "")
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	applyLockFlags(manager, lock)

	projectName, svcName, err := resolveStopTarget(manager, *projectFlags.filePath, *projectFlags.stateDir, flags.Args())
	if err != nil {
		return err
	}

	var record *runtime.ProjectRecord
	if svcName != "" {
		record, err = manager.StopService(projectName, svcName)
	} else {
		record, err = manager.StopProject(projectName)
	}
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}
