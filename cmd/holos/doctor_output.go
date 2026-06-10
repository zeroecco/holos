package main

import (
	"fmt"
	"io"
	"os"
)

func printDoctorReport(report doctorReport) {
	writeDoctorReport(os.Stdout, report)
}

func writeDoctorReport(output io.Writer, report doctorReport) {
	fmt.Fprintf(output, "holos doctor (%s/%s)\n", report.OS, report.Arch)
	fmt.Fprintf(output, "state dir: %s\n\n", report.StateDir)

	writer := newTableWriter(output)
	fmt.Fprintln(writer, "CHECK\tSTATUS\tDETAIL")
	for _, check := range report.Checks {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", check.Name, check.Status, check.Message)
	}
	_ = writer.Flush()
}
