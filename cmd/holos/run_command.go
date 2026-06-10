package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/zeroecco/holos/internal/runtime"
)

func runRun(args []string) error {
	flags := newFlagSet("run")
	stateDir := addStateDirFlag(flags)
	lock := addLockFlags(flags)
	name := flags.String("name", "", "project name (default: derived from image with random suffix)")
	vcpu := flags.Int("vcpu", 0, "vCPU count (default 1)")
	memory := flags.String("memory", "", "memory size, e.g. \"512M\", \"2G\" (default 512M)")
	user := flags.String("user", "", "cloud-init user (default: ubuntu)")
	imageOS := flags.String("image-os", "", "guest OS family for local/custom images (systemd or openrc)")
	dockerfile := flags.String("dockerfile", "", "use a Dockerfile to provision the VM (image arg becomes optional)")
	uefi := flags.Bool("uefi", false, "boot via OVMF (auto-enabled when --device is set)")
	flags.Bool("detach", true, "start in background (kept for symmetry; foreground is not supported)")
	var ports, volumes, devices, packages, runcmd stringList
	flags.Var(&ports, "p", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&ports, "port", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&volumes, "v", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&volumes, "volume", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&devices, "device", "PCI address to pass through, e.g. 0000:01:00.0 (repeatable)")
	flags.Var(&packages, "pkg", "cloud-init package to install (repeatable)")
	flags.Var(&runcmd, "runcmd", "shell command to run on first boot (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	request, err := newRunRequest(flags.Args(), runOptions{
		name:       *name,
		vcpu:       *vcpu,
		memory:     *memory,
		user:       *user,
		imageOS:    *imageOS,
		dockerfile: *dockerfile,
		uefi:       *uefi,
		ports:      []string(ports),
		volumes:    []string(volumes),
		devices:    []string(devices),
		packages:   []string(packages),
		runcmd:     []string(runcmd),
	})
	if err != nil {
		return err
	}

	composePath, err := writeRunComposeFile(*stateDir, request.projectName, request.file)
	if err != nil {
		return err
	}

	project, err := loadProject(composePath, *stateDir)
	if err != nil {
		return fmt.Errorf("synthesise project (see %s):\n%w", composePath, err)
	}

	manager := runtime.NewManager(*stateDir)
	applyLockFlags(manager, lock)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	loginUser := project.Services[request.serviceName].CloudInit.User
	printRunSummary(record, composePath, request.projectName, loginUser)
	return nil
}

type stringList []string

const stringListSeparator = ","

func (s *stringList) String() string { return strings.Join(*s, stringListSeparator) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

var runNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
