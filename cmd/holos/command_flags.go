package main

import (
	"flag"
	"os"
	"time"

	"github.com/zeroecco/holos/internal/runtime"
)

type projectFlags struct {
	filePath *string
	stateDir *string
}

const defaultProjectFileUsage = "path to holos.yaml"

func addProjectFlags(flags *flag.FlagSet, fileUsage string) projectFlags {
	if fileUsage == "" {
		fileUsage = defaultProjectFileUsage
	}
	return projectFlags{
		filePath: flags.String("f", "", fileUsage),
		stateDir: flags.String("state-dir", runtime.DefaultStateDir(), "state directory"),
	}
}

func addStateDirFlag(flags *flag.FlagSet) *string {
	return flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	return flags
}

type lockFlags struct {
	timeout *time.Duration
	noWait  *bool
}

func addLockFlags(flags *flag.FlagSet) lockFlags {
	return lockFlags{
		timeout: flags.Duration("lock-timeout", runtime.DefaultLockWaitTimeout, "maximum time to wait for the project lock"),
		noWait:  flags.Bool("no-wait", false, "fail immediately if the project lock is held"),
	}
}

func applyLockFlags(manager *runtime.Manager, flags lockFlags) {
	manager.SetLockOptions(runtime.LockOptions{
		WaitTimeout: *flags.timeout,
		NoWait:      *flags.noWait,
	})
}
