// Mock qemu-img used by the holos integration test suite.
//
// Supports the "create" subcommand as invoked by holos in two flavours:
//
//   - overlay:        qemu-img create -f qcow2 -F <format> -b <backing> <overlay>
//   - blank volume:   qemu-img create -f qcow2 <path> <sizeBytes>
//
// In both cases the mock creates a tiny sentinel file at the output path.
// We pick the output path as the last positional argument that contains a
// '/' — size arguments are bare integers, so this reliably distinguishes
// them without tracking every qemu-img flag.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "qemu-img: missing subcommand")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		output := pickOutputPath(args[1:])
		if output == "" {
			fmt.Fprintf(os.Stderr, "qemu-img: create: no path-like arg in %v\n", args[1:])
			os.Exit(1)
		}
		if err := os.WriteFile(output, []byte("mock-qcow2"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "qemu-img: create %s: %v\n", output, err)
			os.Exit(1)
		}
	case "info", "check":
	default:
		fmt.Fprintf(os.Stderr, "qemu-img: mock does not implement %q\n", args[0])
		os.Exit(1)
	}
}

// pickOutputPath scans arguments right-to-left and returns the last one
// that looks like a filesystem path. "Path-like" means it contains a '/'
// which cleanly excludes size arguments ("10737418240") without having
// to track flag definitions.
func pickOutputPath(args []string) string {
	for i := len(args) - 1; i >= 0; i-- {
		if strings.ContainsRune(args[i], '/') {
			return args[i]
		}
	}
	return ""
}
