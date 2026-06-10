package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/runtime"
)

func runLogs(args []string) error {
	flags := newFlagSet("logs")
	projectFlags := addProjectFlags(flags, "")
	lines := flags.Int("n", defaultLogTailLines, "number of lines")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return errors.New("logs requires a project name (e.g. \"my-stack\") or a service/instance (e.g. \"vm\", \"vm-0\")")
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	var (
		record *runtime.ProjectRecord
		filter string
	)
	if *projectFlags.filePath == "" {
		if r, ok := lookupProjectRecord(manager, flags.Arg(0)); ok {
			record = r
			if flags.NArg() >= 2 {
				filter = flags.Arg(1)
			}
		}
	}
	if record == nil {
		if flags.NArg() != 1 {
			return errors.New("logs <project> [<service|instance>]  OR  logs [-f file] <service|instance>")
		}
		project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
		if err != nil {
			return err
		}
		record, err = manager.ProjectStatus(project.Name)
		if err != nil {
			return err
		}
		filter = flags.Arg(0)
	}

	matches := logTargets(record, filter)
	if filter != "" {
		if len(matches) == 0 {
			return fmt.Errorf("no service or instance named %q in project %q", filter, record.Name)
		}
	}

	for _, inst := range matches {
		writeLogTarget(os.Stdout, os.Stderr, inst, *lines)
	}
	return nil
}

func writeLogTarget(output io.Writer, warningOutput io.Writer, inst runtime.InstanceRecord, lines int) {
	fmt.Fprintf(output, logHeaderFormat+"\n", inst.Name)
	writeLogTail(output, warningOutput, inst.LogPath, lines)
	fmt.Fprintln(output)
}
