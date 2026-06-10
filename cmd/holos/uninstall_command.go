package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/systemd"
)

const uninstallResultLineFormat = "removed %s unit: %s\n"

func runUninstall(args []string) error {
	flags := newFlagSet("uninstall")
	projectFlags := addProjectFlags(flags, "")
	system := flags.Bool("system", false, "uninstall the system unit instead of --user")
	name := flags.String("name", "", "project name (defaults to the name parsed from -f)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectName, err := uninstallProjectName(*name, *projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}

	res, err := systemd.Uninstall(installScope(*system), projectName)
	if err != nil {
		return err
	}
	writeUninstallResult(os.Stdout, os.Stderr, res)
	return nil
}

func writeUninstallResult(output io.Writer, warningOutput io.Writer, res systemd.Result) {
	fmt.Fprintf(output, uninstallResultLineFormat, res.Scope, res.UnitPath)
	for _, w := range res.Warnings {
		writeWarning(warningOutput, "%s", w)
	}
}

func uninstallProjectName(name, filePath, stateDir string) (string, error) {
	if name != "" {
		if err := compose.ValidateName(name); err != nil {
			return "", fmt.Errorf("invalid --name: %w", err)
		}
		return name, nil
	}

	project, _, err := loadProjectWithPath(filePath, stateDir)
	if err != nil {
		return "", err
	}
	return project.Name, nil
}
