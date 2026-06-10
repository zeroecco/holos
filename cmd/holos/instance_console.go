package main

import (
	"github.com/zeroecco/holos/internal/console"
	"github.com/zeroecco/holos/internal/runtime"
)

func runConsole(args []string) error {
	flags := newFlagSet("console")
	projectFlags := addProjectFlags(flags, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return instanceTargetRequiredError("console")
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	tgt, err := resolveInstanceTarget(manager, *projectFlags.filePath, *projectFlags.stateDir, flags.Args())
	if err != nil {
		return err
	}
	if !instanceIsRunning(tgt.Inst) {
		return instanceNotRunningError(tgt.Inst)
	}
	if !instanceSupportsConsole(tgt.Inst) {
		return instanceMissingConsoleSupportError(tgt.Inst)
	}
	return console.Attach(tgt.Inst.SerialPath)
}
