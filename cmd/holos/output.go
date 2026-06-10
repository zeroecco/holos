package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/zeroecco/holos/internal/runtime"
)

const (
	tablePlaceholder = "-"

	tableMinWidth = 0
	tableTabWidth = 8
	tablePadding  = 2
	tablePadChar  = ' '
	tableFlags    = 0
)

func writeWarning(output io.Writer, format string, args ...any) {
	fmt.Fprintf(output, "warning: "+format+"\n", args...)
}

func printWarning(format string, args ...any) {
	writeWarning(os.Stderr, format, args...)
}

func newTableWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(output, tableMinWidth, tableTabWidth, tablePadding, tablePadChar, tableFlags)
}

func tableValue(value string) string {
	if value == "" {
		return tablePlaceholder
	}
	return value
}

func printProjectStatus(record *runtime.ProjectRecord) {
	writeProjectStatus(os.Stdout, record)
}

func writeProjectStatus(output io.Writer, record *runtime.ProjectRecord) {
	fmt.Fprintf(output, "project: %s\n\n", record.Name)
	for _, svc := range record.Services {
		fmt.Fprintf(output, "service: %s (%d/%d running)\n", svc.Name, svc.RunningCount(), svc.DesiredReplicas)
		writer := newTableWriter(output)
		fmt.Fprintln(writer, "  INSTANCE\tSTATUS\tPID\tPORTS\tLOG")
		for _, inst := range svc.Instances {
			fmt.Fprintln(writer, formatProjectStatusRow(inst))
		}
		_ = writer.Flush()
		fmt.Fprintln(output)
	}
}

func formatProjectStatusRow(inst runtime.InstanceRecord) string {
	return fmt.Sprintf("  %s\t%s\t%d\t%s\t%s",
		inst.Name,
		inst.Status,
		inst.PID,
		tableValue(inst.PortSummary()),
		tableValue(inst.LogPath),
	)
}

func printJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
