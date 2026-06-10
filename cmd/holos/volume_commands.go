package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zeroecco/holos/internal/runtime"
)

func runVolumes(args []string) error {
	if len(args) > 0 && isVolumeRemoveCommand(args[0]) {
		return runVolumeRemove(args[1:])
	}

	flags := newFlagSet("volumes")
	projectFlags := addProjectFlags(flags, "path to holos.yaml (limits output to that one project)")
	jsonOut := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	volumes, err := manager.ListVolumes()
	if err != nil {
		return err
	}
	if *projectFlags.filePath != "" {
		project, err := loadProject(*projectFlags.filePath, *projectFlags.stateDir)
		if err != nil {
			return err
		}
		volumes = filterVolumesByProject(volumes, project.Name)
	}

	if *jsonOut {
		return printJSON(volumes)
	}
	return writeVolumesTable(os.Stdout, volumes)
}

func isVolumeRemoveCommand(arg string) bool {
	return arg == "rm" || arg == "remove"
}

func runVolumeRemove(args []string) error {
	flags := newFlagSet("volumes rm")
	stateDir := addStateDirFlag(flags)
	lock := addLockFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: holos volumes rm <project> <volume>")
	}

	manager := runtime.NewManager(*stateDir)
	applyLockFlags(manager, lock)
	return manager.RemoveVolume(flags.Arg(0), flags.Arg(1))
}

func filterVolumesByProject(volumes []runtime.VolumeInfo, project string) []runtime.VolumeInfo {
	filtered := make([]runtime.VolumeInfo, 0, len(volumes))
	for _, volume := range volumes {
		if volume.Project == project {
			filtered = append(filtered, volume)
		}
	}
	return filtered
}

func writeVolumesTable(output io.Writer, volumes []runtime.VolumeInfo) error {
	if len(volumes) == 0 {
		fmt.Fprintln(output, "no named volumes")
		return nil
	}

	writer := newTableWriter(output)
	fmt.Fprintln(writer, "PROJECT\tVOLUME\tSIZE_BYTES\tATTACHED_TO\tPATH")
	for _, volume := range volumes {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\n",
			volume.Project,
			volume.Name,
			volume.SizeBytes,
			formatVolumeAttachments(volume.Attachments),
			volume.Path,
		)
	}
	return writer.Flush()
}

func formatVolumeAttachments(attachments []runtime.VolumeAttachmentInfo) string {
	if len(attachments) == 0 {
		return tablePlaceholder
	}
	out := make([]string, len(attachments))
	for i, attachment := range attachments {
		out[i] = fmt.Sprintf("%s:%s", attachment.Instance, attachment.Status)
	}
	return strings.Join(out, ",")
}
