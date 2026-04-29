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

	switch args[0] {
	case "up":
		return runUp(args[1:])
	case "run":
		return runRun(args[1:])
	case "down":
		return runDown(args[1:])
	case "ps":
		return runPS(args[1:])
	case "start":
		return runStart(args[1:])
	case "stop":
		return runStop(args[1:])
	case "console":
		return runConsole(args[1:])
	case "exec":
		return runExec(args[1:])
	case "logs":
		return runLogs(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "pull":
		return runPull(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "images":
		return runImages(args[1:])
	case "devices":
		return runDevices(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "install":
		return runInstall(args[1:])
	case "uninstall":
		return runUninstall(args[1:])
	case "import":
		return runImport(args[1:])
	case "version", "--version", "-v":
		return runVersion(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}
