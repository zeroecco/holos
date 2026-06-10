package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/runtime"
)

func runUp(args []string) error {
	flags := newFlagSet("up")
	projectFlags := addProjectFlags(flags, "")
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	applyLockFlags(manager, lock)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runDown(args []string) error {
	flags := newFlagSet("down")
	projectFlags := addProjectFlags(flags, "")
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	var projectName string
	if flags.NArg() > 0 {
		projectName = flags.Arg(0)
		if err := compose.ValidateName(projectName); err != nil {
			return fmt.Errorf("invalid project name: %w", err)
		}
	} else {
		project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
		if err != nil {
			return err
		}
		projectName = project.Name
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	applyLockFlags(manager, lock)
	if err := manager.Down(projectName); err != nil {
		return err
	}

	writeProjectRemoved(os.Stdout, projectName)
	return nil
}

func writeProjectRemoved(output io.Writer, projectName string) {
	fmt.Fprintf(output, "project %q removed\n", projectName)
}
