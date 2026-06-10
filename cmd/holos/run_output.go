package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zeroecco/holos/internal/runtime"
)

type runNextStep struct {
	Command     string
	Description string
}

const (
	runNextStepCommandWidth = 7
	runNextStepIndent       = "  "
	runNextStepCommentGap   = "     "
)

var runNextSteps = []runNextStep{
	{Command: "exec", Description: "interactive shell over ssh (recommended)"},
	{Command: "console", Description: "serial console for boot/kernel logs"},
	{Command: "logs", Description: "console.log tail"},
	{Command: "down"},
}

func printRunSummary(record *runtime.ProjectRecord, composePath, projectName, loginUser string) {
	writeRunSummary(os.Stdout, record, composePath, projectName, loginUser)
}

func writeRunSummary(output io.Writer, record *runtime.ProjectRecord, composePath, projectName, loginUser string) {
	writeProjectStatus(output, record)
	fmt.Fprintf(output, "compose file: %s\n", composePath)
	fmt.Fprintf(output, "login user:   %s (cloud-init may take ~30s on first boot)\n", loginUser)
	fmt.Fprintln(output)
	fmt.Fprintln(output, "next steps:")
	for _, step := range runNextSteps {
		writeRunNextStep(output, step, projectName)
	}
}

func printRunNextStep(step runNextStep, projectName string) {
	writeRunNextStep(os.Stdout, step, projectName)
}

func writeRunNextStep(output io.Writer, step runNextStep, projectName string) {
	fmt.Fprintln(output, formatRunNextStep(step, projectName))
}

func formatRunNextStep(step runNextStep, projectName string) string {
	line := fmt.Sprintf("%sholos %-*s %s", runNextStepIndent, runNextStepCommandWidth, step.Command, projectName)
	if step.Description != "" {
		line += runNextStepCommentGap + "# " + step.Description
	}
	return line
}
