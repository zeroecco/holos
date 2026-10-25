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
	dryRun := flags.Bool("dry-run", false, "preview service changes without changing VMs")
	recreate := flags.Bool("recreate", false, "allow replacement/removal of changed services and their root disks")
	strict := flags.Bool("strict", false, "reject ignored Compose settings")
	locked := flags.Bool("locked", false, "require holos.images.lock and reject image drift")
	lockfile := flags.String("lockfile", "", "image lockfile path (defaults to holos.images.lock beside the compose file)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, composePath, err := loadResolvedProject(*projectFlags.filePath, *projectFlags.stateDir, *strict)
	if err != nil {
		return err
	}
	if err := verifyProjectImageLockMode(composePath, project, *locked, *lockfile); err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	applyLockFlags(manager, lock)
	options := runtime.UpOptions{
		DryRun:   *dryRun,
		Recreate: *recreate,
		Report: func(changes []runtime.ServiceChange) {
			for _, change := range changes {
				fmt.Fprintf(os.Stderr, "%s: %s (%s)\n", change.Service, change.Action, change.Reason)
			}
		},
	}
	record, err := manager.UpWithOptions(project, options)
	if err != nil {
		return err
	}

	if *dryRun {
		return nil
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
		project, _, err := loadComposeFile(*projectFlags.filePath)
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
