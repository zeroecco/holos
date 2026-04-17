// Mock qemu-img used by the holos integration test suite.
//
// Supports only the "create" subcommand as invoked by holos:
//
//	qemu-img create -f qcow2 -F <format> -b <backing> <overlay>
//
// The mock creates a zero-byte file at the overlay path. The real qcow2
// format is irrelevant because the paired mock qemu-system never reads it.
package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "qemu-img: missing subcommand")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		overlay := args[len(args)-1]
		if err := os.WriteFile(overlay, []byte("mock-qcow2"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "qemu-img: create %s: %v\n", overlay, err)
			os.Exit(1)
		}
	case "info", "check":
	default:
		fmt.Fprintf(os.Stderr, "qemu-img: mock does not implement %q\n", args[0])
		os.Exit(1)
	}
}
