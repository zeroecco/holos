package main

import (
	"fmt"
	"os"

	"github.com/zeroecco/holos/internal/compose"
	"gopkg.in/yaml.v3"
)

const (
	importStdoutOutput = "-"
	importOutputPerm   = os.FileMode(0o644)
)

func importOutputIsStdout(output string) bool {
	return output == "" || output == importStdoutOutput
}

func writeImportOutput(output string, file compose.File, warnings []string) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	for _, w := range warnings {
		printWarning("%s", w)
	}
	if importOutputIsStdout(output) {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(output, data, importOutputPerm); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	fmt.Fprintln(os.Stderr, formatImportOutputSummary(output, len(file.Services)))
	return nil
}

func formatImportOutputSummary(output string, serviceCount int) string {
	return fmt.Sprintf("wrote %s (%d service(s))", output, serviceCount)
}
