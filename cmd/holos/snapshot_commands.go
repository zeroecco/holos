package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/runtime"
)

func runSnapshots(args []string) error {
	if len(args) > 0 && args[0] == "create" {
		return runSnapshotCreate(args[1:])
	}
	return fmt.Errorf("usage: holos snapshots create <project> <instance> <snapshot>")
}

func runSnapshotCreate(args []string) error {
	flags := newFlagSet("snapshots create")
	stateDir := addStateDirFlag(flags)
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return fmt.Errorf("usage: holos snapshots create <project> <instance> <snapshot>")
	}

	manager := runtime.NewManager(*stateDir)
	applyLockFlags(manager, lock)
	return manager.SnapshotInstanceRoot(flags.Arg(0), flags.Arg(1), flags.Arg(2))
}
