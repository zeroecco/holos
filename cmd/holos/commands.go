package main

import (
	"fmt"
	"os"
)

type command struct {
	run         func([]string) error
	usage       string
	description string
}

const (
	usageLineIndent = "  "
	usageTextWidth  = 56
)

var commandOrder = []string{
	"up",
	"run",
	"down",
	"ps",
	"start",
	"stop",
	"console",
	"exec",
	"logs",
	"inspect",
	"validate",
	"pull",
	"verify",
	"images",
	"snapshots",
	"volumes",
	"devices",
	"doctor",
	"install",
	"uninstall",
	"import",
	"completion",
	"version",
}

var commands = map[string]command{
	"up": {
		run:         runUp,
		usage:       "holos up [-f holos.yaml] [--locked] [--lockfile path] [--lock-timeout 5m|--no-wait]",
		description: "start all services",
	},
	"run": {
		run:         runRun,
		usage:       "holos run [flags] <image> [-- cmd...]",
		description: "launch a one-off VM from an image (no compose file)",
	},
	"down": {
		run:         runDown,
		usage:       "holos down [-f holos.yaml]",
		description: "stop and remove all services",
	},
	"ps": {
		run:         runPS,
		usage:       "holos ps [-f holos.yaml]",
		description: "list running projects (-f narrows to one)",
	},
	"start": {
		run:         runStart,
		usage:       "holos start [-f holos.yaml] [svc]",
		description: "start a stopped service or all services",
	},
	"stop": {
		run:         runStop,
		usage:       "holos stop [-f holos.yaml] [svc]",
		description: "stop a service or all services",
	},
	"console": {
		run:         runConsole,
		usage:       "holos console [-f holos.yaml] <inst>",
		description: "attach serial console to an instance",
	},
	"exec": {
		run:         runExec,
		usage:       "holos exec [-f holos.yaml] <inst> [cmd...]",
		description: "ssh into an instance (project's generated key)",
	},
	"logs": {
		run:         runLogs,
		usage:       "holos logs [-f holos.yaml] <svc|inst>",
		description: "show logs for a service (all replicas) or one instance",
	},
	"inspect": {
		run:         runInspect,
		usage:       "holos inspect [-f holos.yaml] [project|instance]",
		description: "inspect project or instance state as JSON",
	},
	"validate": {
		run:         runValidate,
		usage:       "holos validate [-f holos.yaml] [--capacity] [--network]",
		description: "validate compose file",
	},
	"pull": {
		run:         runPull,
		usage:       "holos pull <image>",
		description: "pull a cloud image (e.g. alpine, ubuntu:noble)",
	},
	"verify": {
		run:         runVerify,
		usage:       "holos verify <image>|--all",
		description: "verify cached image checksums",
	},
	"images": {
		run:         runImages,
		usage:       "holos images | holos images lock -f holos.yaml [-o holos.images.lock]",
		description: "list available images or write an image lockfile",
	},
	"snapshots": {
		run:         runSnapshots,
		usage:       "holos snapshots {create|list|rm|restore|export} ...",
		description: "manage stopped instance root snapshots",
	},
	"volumes": {
		run:         runVolumes,
		usage:       "holos volumes [list|rm|export|snapshot|snapshots|snapshot-rm|snapshot-restore|snapshot-export|resize] ...",
		description: "list, remove, export, snapshot, or resize named volumes",
	},
	"devices": {
		run:         runDevices,
		usage:       "holos devices [--gpu]",
		description: "list PCI devices and IOMMU groups",
	},
	"doctor": {
		run:         runDoctor,
		usage:       "holos doctor [--json]",
		description: "check host dependencies and state dir access",
	},
	"install": {
		run:         runInstall,
		usage:       "holos install [-f holos.yaml] [--system] [--enable]",
		description: "emit a systemd unit so the project comes back up on reboot",
	},
	"uninstall": {
		run:         runUninstall,
		usage:       "holos uninstall [-f holos.yaml] [--system]",
		description: "remove the systemd unit written by 'holos install'",
	},
	"import": {
		run:         runImport,
		usage:       "holos import [vm...] [--all] [--xml file] [--connect uri] [-o file]",
		description: "convert virsh-defined VMs into a holos.yaml",
	},
	"completion": {
		run:         runCompletion,
		usage:       "holos completion <bash|zsh|fish>",
		description: "print shell completion script",
	},
	"version": {
		run:         runVersion,
		usage:       "holos version [--short]",
		description: "print build version, commit, and platform",
	},
}

var commandAliases = map[string]string{
	"--version": "version",
	"-v":        "version",
}

func resolveCommand(name string) (command, bool) {
	if canonical, ok := commandAliases[name]; ok {
		name = canonical
	}
	command, ok := commands[name]
	return command, ok
}

func usage() {
	fmt.Fprintln(os.Stderr, "holos - docker compose for KVM")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Usage:")
	for _, name := range commandOrder {
		command := commands[name]
		fmt.Fprintln(os.Stderr, formatUsageLine(command))
	}
}

func formatUsageLine(command command) string {
	return fmt.Sprintf("%s%-*s %s", usageLineIndent, usageTextWidth, command.usage, command.description)
}
