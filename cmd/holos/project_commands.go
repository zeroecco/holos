package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/runtime"
)

func runUp(args []string) error {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runDown(args []string) error {
	flags := flag.NewFlagSet("down", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
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
		project, err := loadProject(*filePath, *stateDir)
		if err != nil {
			return err
		}
		projectName = project.Name
	}

	manager := runtime.NewManager(*stateDir)
	if err := manager.Down(projectName); err != nil {
		return err
	}

	fmt.Printf("project %q removed\n", projectName)
	return nil
}

func runPS(args []string) error {
	flags := flag.NewFlagSet("ps", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml (limits output to that one project)")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	jsonOut := flags.Bool("json", false, "emit JSON")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	var (
		projects []*runtime.ProjectRecord
		err      error
	)
	if *filePath != "" {
		project, perr := loadProject(*filePath, *stateDir)
		if perr != nil {
			return perr
		}
		record, perr := manager.ProjectStatus(project.Name)
		if perr != nil {
			return perr
		}
		projects = []*runtime.ProjectRecord{record}
	} else {
		projects, err = manager.ListProjects()
		if err != nil {
			return err
		}
	}

	if *jsonOut {
		return printJSON(projects)
	}
	if len(projects) == 0 {
		fmt.Println("no running projects")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "PROJECT\tSERVICE\tDESIRED\tRUNNING\tPORTS")
	for _, project := range projects {
		for _, svc := range project.Services {
			fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n",
				project.Name,
				svc.Name,
				svc.DesiredReplicas,
				svc.RunningCount(),
				servicePorts(svc),
			)
		}
	}
	return writer.Flush()
}

func runStart(args []string) error {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	if flags.NArg() > 0 {
		svcName := flags.Arg(0)
		if _, ok := project.Services[svcName]; !ok {
			return fmt.Errorf("service %q not found in project %q", svcName, project.Name)
		}
		for name := range project.Services {
			if name != svcName {
				delete(project.Services, name)
			}
		}
		project.ServiceOrder = []string{svcName}
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runStop(args []string) error {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	var record *runtime.ProjectRecord
	if flags.NArg() > 0 {
		record, err = manager.StopService(project.Name, flags.Arg(0))
	} else {
		record, err = manager.StopProject(project.Name)
	}
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}
