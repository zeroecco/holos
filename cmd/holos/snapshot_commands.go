package main

import (
	"fmt"

	"github.com/zeroecco/holos/internal/runtime"
)

func runSnapshots(args []string) error {
	if len(args) > 0 && args[0] == "create" {
		return runSnapshotCreate(args[1:])
	}
	if len(args) > 0 && args[0] == "list" {
		return runSnapshotList(args[1:])
	}
	if len(args) > 0 && (args[0] == "rm" || args[0] == "remove") {
		return runSnapshotRemove(args[1:])
	}
	return fmt.Errorf("usage: holos snapshots {create|list|rm} ...")
}

func runSnapshotList(args []string) error {
	flags := newFlagSet("snapshots list")
	stateDir := addStateDirFlag(flags)
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: holos snapshots list <project> <instance>")
	}
	m := runtime.NewManager(*stateDir)
	applyLockFlags(m, lock)
	snapshots, err := m.ListInstanceSnapshots(flags.Arg(0), flags.Arg(1))
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		fmt.Println(snapshot.Name)
	}
	return nil
}

func runSnapshotRemove(args []string) error {
	flags := newFlagSet("snapshots rm")
	stateDir := addStateDirFlag(flags)
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 3 {
		return fmt.Errorf("usage: holos snapshots rm <project> <instance> <snapshot>")
	}
	m := runtime.NewManager(*stateDir)
	applyLockFlags(m, lock)
	return m.RemoveInstanceSnapshot(flags.Arg(0), flags.Arg(1), flags.Arg(2))
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
