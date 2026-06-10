package main

import (
	"errors"
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
		return command.run(args[1:])
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}
