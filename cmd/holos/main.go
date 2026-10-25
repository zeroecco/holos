package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// Build metadata is overwritten at link time by goreleaser via -ldflags
// "-X main.version=...". Plain `go build` keeps these defaults and
// runVersion supplements them with Go's embedded VCS metadata.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "holos: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}

	if command, ok := resolveCommand(args[0]); ok {
		if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
			fmt.Fprintln(os.Stderr, command.usage)
			fmt.Fprintln(os.Stderr, command.description)
			fmt.Fprintln(os.Stderr)
			if command.synopsisOnlyHelp {
				return nil
			}
		}
		err := command.run(args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) == 2 {
			if _, ok := resolveCommand(args[1]); !ok {
				return fmt.Errorf("unknown command %q", args[1])
			}
			return run([]string{args[1], "-h"})
		}
		if len(args) > 2 {
			return fmt.Errorf("usage: holos help [command]")
		}
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}
