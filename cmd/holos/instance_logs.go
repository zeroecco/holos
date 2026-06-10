package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	defaultLogTailLines = 50
	logHeaderFormat     = "==> %s <=="
	logLineSeparator    = "\n"
)

func printLogTail(path string, lines int) {
	writeLogTail(os.Stdout, os.Stderr, path, lines)
}

func writeLogTail(output io.Writer, warningOutput io.Writer, path string, lines int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(warningOutput, "  (cannot read log: %v)\n", err)
		return
	}

	for _, line := range logTailLines(data, lines) {
		fmt.Fprintln(output, line)
	}
}

func logTailLines(data []byte, lines int) []string {
	if lines <= 0 {
		return nil
	}

	allLines := strings.Split(strings.TrimRight(string(data), logLineSeparator), logLineSeparator)
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	return allLines[start:]
}
