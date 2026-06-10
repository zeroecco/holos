package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zeroecco/holos/internal/systemd"
)

const (
	installUserActivationCommand   = "systemctl --user enable --now"
	installSystemActivationCommand = "sudo systemctl enable --now"
	installSystemctlMissingNote    = "note: systemctl not found on PATH; unit is on disk but not loaded"
	installDryRunOutputFormat      = "# would write to: %s\n%s"
	installResultLineFormat        = "installed %s unit: %s\n"
	installActivationHintFormat    = "to activate at boot: %s holos-%s.service\n"
)

func runInstall(args []string) error {
	flags := newFlagSet("install")
	projectFlags := addProjectFlags(flags, "")
	system := flags.Bool("system", false, "install system-wide (/etc/systemd/system) instead of --user")
	runAs := flags.String("user", "", "with --system, run the service as this user")
	enable := flags.Bool("enable", false, "run systemctl enable --now after installing")
	dryRun := flags.Bool("dry-run", false, "print the unit content without writing to disk")
	if err := flags.Parse(args); err != nil {
		return err
	}

	stateDirExplicit := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "state-dir" {
			stateDirExplicit = true
		}
	})
	if installSystemUserRequiresExplicitStateDir(*system, *runAs, stateDirExplicit) {
		return installSystemUserStateDirError(*runAs)
	}

	project, absCompose, err := loadProjectWithPath(*projectFlags.filePath, *projectFlags.stateDir)
	if err != nil {
		return err
	}
	holosPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve holos binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(holosPath); err == nil {
		holosPath = resolved
	}
	absState, err := filepath.Abs(*projectFlags.stateDir)
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}

	scope := installScope(*system)
	spec := systemd.UnitSpec{
		Project:     project.Name,
		ComposeFile: absCompose,
		HolosBinary: holosPath,
		StateDir:    absState,
		Scope:       scope,
		User:        *runAs,
	}
	if *dryRun {
		path, content, err := systemd.Render(spec)
		if err != nil {
			return err
		}
		writeInstallDryRun(os.Stdout, path, content)
		return nil
	}

	res, err := systemd.Install(spec, *enable)
	if err != nil {
		return err
	}
	writeInstallResult(os.Stdout, os.Stderr, res, *enable, project.Name)
	return nil
}

func writeInstallDryRun(output io.Writer, path string, content string) {
	fmt.Fprintf(output, installDryRunOutputFormat, path, content)
}

func writeInstallResult(output io.Writer, warningOutput io.Writer, res systemd.Result, enable bool, projectName string) {
	fmt.Fprintf(output, installResultLineFormat, res.Scope, res.UnitPath)
	if res.SystemctlMissing {
		fmt.Fprintln(output, installSystemctlMissingNote)
	}
	for _, w := range res.Warnings {
		writeWarning(warningOutput, "%s", w)
	}
	if installShouldPrintActivationHint(enable, res.SystemctlMissing) {
		fmt.Fprintf(output, installActivationHintFormat, installActivationCommand(res.Scope), projectName)
	}
}

func installShouldPrintActivationHint(enable bool, systemctlMissing bool) bool {
	return !enable && !systemctlMissing
}

func installScope(system bool) systemd.Scope {
	if system {
		return systemd.ScopeSystem
	}
	return systemd.ScopeUser
}

func installSystemUserRequiresExplicitStateDir(system bool, runAs string, stateDirExplicit bool) bool {
	return system && runAs != "" && !stateDirExplicit
}

func installSystemUserStateDirError(runAs string) error {
	return fmt.Errorf(
		"install --system --user %s requires --state-dir pointing at a directory %[1]s can read and write; "+
			"holos locks the state tree to 0700 so running as %[1]s would otherwise fail at start",
		runAs)
}

func installActivationCommand(scope systemd.Scope) string {
	if scope == systemd.ScopeSystem {
		return installSystemActivationCommand
	}
	return installUserActivationCommand
}
