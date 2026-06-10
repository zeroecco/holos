package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/runtime"
)

func runPS(args []string) error {
	flags := newFlagSet("ps")
	projectFlags := addProjectFlags(flags, "path to holos.yaml (limits output to that one project)")
	jsonOut := flags.Bool("json", false, "emit JSON")
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	applyLockFlags(manager, lock)
	projects, err := loadProjectStatuses(manager, *projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}

	if *jsonOut {
		return printJSON(projects)
	}
	return printProjectsTable(projects)
}

func loadProjectStatuses(manager *runtime.Manager, filePath, stateDir string) ([]*runtime.ProjectRecord, error) {
	if filePath == "" {
		return manager.ListProjects()
	}

	project, err := loadProject(filePath, stateDir)
	if err != nil {
		return nil, err
	}
	record, err := manager.ProjectStatus(project.Name)
	if err != nil {
		return nil, err
	}
	return []*runtime.ProjectRecord{record}, nil
}

func printProjectsTable(projects []*runtime.ProjectRecord) error {
	return writeProjectsTable(os.Stdout, projects)
}

func writeProjectsTable(output io.Writer, projects []*runtime.ProjectRecord) error {
	if len(projects) == 0 {
		fmt.Fprintln(output, "no running projects")
		return nil
	}

	writer := newTableWriter(output)
	fmt.Fprintln(writer, "PROJECT\tSERVICE\tDESIRED\tRUNNING\tPORTS")
	for _, project := range projects {
		for _, svc := range project.Services {
			fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n",
				project.Name,
				svc.Name,
				svc.DesiredReplicas,
				svc.RunningCount(),
				svc.PortSummary(),
			)
		}
	}
	return writer.Flush()
}
