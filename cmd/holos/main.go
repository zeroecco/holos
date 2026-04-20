package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/console"
	"github.com/zeroecco/holos/internal/images"
	"github.com/zeroecco/holos/internal/runtime"
	"github.com/zeroecco/holos/internal/systemd"
	"github.com/zeroecco/holos/internal/vfio"
	"github.com/zeroecco/holos/internal/virtimport"
	"gopkg.in/yaml.v3"
)

// Build metadata is overwritten at link time by goreleaser via -ldflags
// "-X main.version=...". The defaults below are what `go build` and
// `go install` produce: a "dev" tag plus whatever VCS info Go's runtime
// debug.ReadBuildInfo can recover (commit hash + dirty flag).
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
	case "images":
		return runImages(args[1:])
	case "devices":
		return runDevices(args[1:])
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

func runUp(args []string) error {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runDown(args []string) error {
	flags := flag.NewFlagSet("down", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	var projectName string

	// If a positional arg is given, use it as the project name directly.
	if flags.NArg() > 0 {
		projectName = flags.Arg(0)
	} else {
		project, err := loadProject(*filePath, *stateDir)
		if err != nil {
			return err
		}
		projectName = project.Name
	}

	manager := runtime.NewManager(*stateDir)
	if err := manager.Down(projectName); err != nil {
		return err
	}

	fmt.Printf("project %q removed\n", projectName)
	return nil
}

func runPS(args []string) error {
	flags := flag.NewFlagSet("ps", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	jsonOut := flags.Bool("json", false, "emit JSON")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	projects, err := manager.ListProjects()
	if err != nil {
		return err
	}

	if *jsonOut {
		return printJSON(projects)
	}

	if len(projects) == 0 {
		fmt.Println("no running projects")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "PROJECT\tSERVICE\tDESIRED\tRUNNING\tPORTS")
	for _, project := range projects {
		for _, svc := range project.Services {
			ports := servicePorts(svc)
			fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n",
				project.Name,
				svc.Name,
				svc.DesiredReplicas,
				svc.RunningCount(),
				ports,
			)
		}
	}
	return writer.Flush()
}

func runStart(args []string) error {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	if flags.NArg() > 0 {
		svcName := flags.Arg(0)
		if _, ok := project.Services[svcName]; !ok {
			return fmt.Errorf("service %q not found in project %q", svcName, project.Name)
		}
		// Filter to just the requested service so Up only reconciles it.
		for name := range project.Services {
			if name != svcName {
				delete(project.Services, name)
			}
		}
		project.ServiceOrder = []string{svcName}
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runStop(args []string) error {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)

	var record *runtime.ProjectRecord
	if flags.NArg() > 0 {
		record, err = manager.StopService(project.Name, flags.Arg(0))
	} else {
		record, err = manager.StopProject(project.Name)
	}
	if err != nil {
		return err
	}

	printProjectStatus(record)
	return nil
}

func runLogs(args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	lines := flags.Int("n", 50, "number of lines")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("logs requires a service name")
	}

	serviceName := flags.Arg(0)

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.ProjectStatus(project.Name)
	if err != nil {
		return err
	}

	for _, svc := range record.Services {
		if svc.Name != serviceName {
			continue
		}
		for _, inst := range svc.Instances {
			fmt.Printf("==> %s <==\n", inst.Name)
			printLogTail(inst.LogPath, *lines)
			fmt.Println()
		}
		return nil
	}

	return fmt.Errorf("service %q not found in project %q", serviceName, project.Name)
}

// runExec opens an ssh session to a running instance.
//
// Layout: holos exec [-f holos.yaml] [-u user] <instance> [-- cmd ...]
//
//   - The project is resolved from -f (or auto-discovered in cwd) so
//     we know which project owns the instance's keypair and cloud-init
//     user.
//   - When no command is given we allocate a TTY and drop the operator
//     into a login shell; with a command we pass it verbatim to ssh and
//     inherit stdin so pipes work ("holos exec db-0 psql < dump.sql").
//   - Host-key checks are disabled because guests are ephemeral and
//     their fingerprints rotate on every `down`/`up`. Keys live on
//     /dev/null so we never pollute the operator's known_hosts.
func runExec(args []string) error {
	flags := flag.NewFlagSet("exec", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	user := flags.String("u", "", "override login user (default: service's cloud-init user)")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return errors.New("exec requires an instance name (e.g. web-0)")
	}

	instanceName := flags.Arg(0)
	cmd := flags.Args()[1:]

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	inst, svcName, err := manager.FindInstance(project.Name, instanceName)
	if err != nil {
		return err
	}
	if inst.Status != "running" {
		return fmt.Errorf("instance %q is %s", instanceName, inst.Status)
	}
	if inst.SSHPort == 0 {
		return fmt.Errorf("instance %q has no ssh port (created before exec support; recreate the stack)", instanceName)
	}

	loginUser := *user
	if loginUser == "" {
		if svc, ok := project.Services[svcName]; ok && svc.CloudInit.User != "" {
			loginUser = svc.CloudInit.User
		} else {
			loginUser = "ubuntu"
		}
	}

	keyPath, err := manager.ProjectSSHKeyPath(project.Name)
	if err != nil {
		return err
	}

	sshArgs := []string{
		"-i", keyPath,
		"-p", fmt.Sprintf("%d", inst.SSHPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
	}
	if len(cmd) == 0 {
		sshArgs = append(sshArgs, "-t")
	}
	sshArgs = append(sshArgs, fmt.Sprintf("%s@127.0.0.1", loginUser))
	sshArgs = append(sshArgs, cmd...)

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh client not found in PATH: %w", err)
	}

	// Inherit file descriptors directly so the user's terminal, signals,
	// and tty modes flow through to the remote shell. We use
	// syscall.Exec to replace the holos process entirely, which makes
	// Ctrl-C and exit codes behave exactly like a direct ssh call.
	argv := append([]string{sshBin}, sshArgs...)
	return syscall.Exec(sshBin, argv, os.Environ())
}

func runConsole(args []string) error {
	flags := flag.NewFlagSet("console", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("console requires an instance name (e.g. web-0)")
	}

	instanceName := flags.Arg(0)

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.ProjectStatus(project.Name)
	if err != nil {
		return err
	}

	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Name == instanceName {
				if inst.Status != "running" {
					return fmt.Errorf("instance %q is %s", instanceName, inst.Status)
				}
				if inst.SerialPath == "" {
					return fmt.Errorf("instance %q has no serial console (created before console support)", instanceName)
				}
				return console.Attach(inst.SerialPath)
			}
		}
	}

	return fmt.Errorf("instance %q not found in project %q", instanceName, project.Name)
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, err := loadProject(*filePath, *stateDir)
	if err != nil {
		return err
	}

	fmt.Printf("project: %s\n", project.Name)
	fmt.Printf("spec_hash: %s\n", project.SpecHash)
	fmt.Printf("services: %d\n", len(project.Services))
	fmt.Printf("order: %v\n", project.ServiceOrder)
	fmt.Println()

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "SERVICE\tIMAGE\tREPLICAS\tVCPU\tMEMORY")
	for _, name := range project.ServiceOrder {
		m := project.Services[name]
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%dMB\n",
			name,
			filepath.Base(m.Image),
			m.Replicas,
			m.VM.VCPU,
			m.VM.MemoryMB,
		)
	}
	_ = writer.Flush()

	fmt.Printf("\nnetwork: %s (mcast %s:%d)\n",
		project.Network.Subnet,
		project.Network.MulticastGroup,
		project.Network.MulticastPort,
	)
	fmt.Println("hosts:")
	for host, ip := range project.Network.Hosts {
		fmt.Printf("  %s -> %s\n", host, ip)
	}

	return nil
}

func runPull(args []string) error {
	flags := flag.NewFlagSet("pull", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("pull requires an image name (e.g. alpine, ubuntu:noble)")
	}

	cacheDir := images.DefaultCacheDir(*stateDir)
	path, format, err := images.Pull(flags.Arg(0), cacheDir)
	if err != nil {
		return err
	}

	fmt.Printf("image: %s\n", path)
	fmt.Printf("format: %s\n", format)
	return nil
}

func runImages(_ []string) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTAG\tFORMAT")
	for _, img := range images.ListAvailable() {
		name := img.Name
		if img.Default {
			name += " *"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", name, img.Tag, img.Format)
	}
	return writer.Flush()
}

func runDevices(args []string) error {
	flags := flag.NewFlagSet("devices", flag.ContinueOnError)
	gpuOnly := flags.Bool("gpu", false, "show only GPU devices")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *gpuOnly {
		gpus, err := vfio.ListGPUs()
		if err != nil {
			return err
		}
		if len(gpus) == 0 {
			fmt.Println("no GPUs found")
			return nil
		}

		writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(writer, "PCI\tTYPE\tVENDOR:DEVICE\tDRIVER\tIOMMU")
		for _, gpu := range gpus {
			fmt.Fprintf(writer, "%s\t%s\t%s:%s\t%s\t%d\n",
				gpu.Address, gpu.ClassName, gpu.Vendor, gpu.DeviceID, gpu.Driver, gpu.IOMMUGroup)
		}
		return writer.Flush()
	}

	groups, err := vfio.ListIOMMUGroups()
	if err != nil {
		return err
	}

	for _, group := range groups {
		fmt.Printf("IOMMU Group %d:\n", group.ID)
		for _, dev := range group.Devices {
			driver := dev.Driver
			if driver == "" {
				driver = "-"
			}
			fmt.Printf("  %s  %s  %s:%s  [%s]\n",
				dev.Address, dev.ClassName, dev.Vendor, dev.DeviceID, driver)
		}
	}
	return nil
}

// runInstall writes a systemd unit so the project comes back up after
// a host reboot. By default we install a --user unit (no sudo needed);
// --system installs to /etc/systemd/system for pre-login boot.
//
// Flags:
//
//	-f <path>      compose file (auto-discovered if omitted)
//	--system       install system-wide instead of --user
//	--user <name>  only with --system: User= directive in the unit
//	--enable       also enable --now so it starts immediately
//	--dry-run      print the unit to stdout and exit (no write)
//
// We resolve every path to an absolute before handing it to the
// generator; systemd units run with almost no environment, so relative
// paths or a PATH-dependent "holos" binary would break silently on
// reboot.
func runInstall(args []string) error {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	system := flags.Bool("system", false, "install system-wide (/etc/systemd/system) instead of --user")
	runAs := flags.String("user", "", "with --system, run the service as this user")
	enable := flags.Bool("enable", false, "run systemctl enable --now after installing")
	dryRun := flags.Bool("dry-run", false, "print the unit content without writing to disk")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	project, absCompose, err := loadProjectWithPath(*filePath, *stateDir)
	if err != nil {
		return err
	}

	holosPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve holos binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(holosPath); err == nil {
		holosPath = resolved
	}

	absState, err := filepath.Abs(*stateDir)
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}

	scope := systemd.ScopeUser
	if *system {
		scope = systemd.ScopeSystem
	}

	spec := systemd.UnitSpec{
		Project:     project.Name,
		ComposeFile: absCompose,
		HolosBinary: holosPath,
		StateDir:    absState,
		Scope:       scope,
		User:        *runAs,
	}

	if *dryRun {
		path, content, err := systemd.Render(spec)
		if err != nil {
			return err
		}
		fmt.Printf("# would write to: %s\n%s", path, content)
		return nil
	}

	res, err := systemd.Install(spec, *enable)
	if err != nil {
		return err
	}
	fmt.Printf("installed %s unit: %s\n", res.Scope, res.UnitPath)
	if res.SystemctlMissing {
		fmt.Println("note: systemctl not found on PATH; unit is on disk but not loaded")
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if !*enable && !res.SystemctlMissing {
		hint := "systemctl --user enable --now"
		if scope == systemd.ScopeSystem {
			hint = "sudo systemctl enable --now"
		}
		fmt.Printf("to activate at boot: %s holos-%s.service\n", hint, project.Name)
	}
	return nil
}

// runUninstall removes the systemd unit written by `holos install`.
// It does not stop the project — operators that want a clean teardown
// should `holos down` separately.
func runUninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	system := flags.Bool("system", false, "uninstall the system unit instead of --user")
	name := flags.String("name", "", "project name (defaults to the name parsed from -f)")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectName := *name
	if projectName == "" {
		project, _, err := loadProjectWithPath(*filePath, *stateDir)
		if err != nil {
			return err
		}
		projectName = project.Name
	}

	scope := systemd.ScopeUser
	if *system {
		scope = systemd.ScopeSystem
	}

	res, err := systemd.Uninstall(scope, projectName)
	if err != nil {
		return err
	}
	fmt.Printf("removed %s unit: %s\n", res.Scope, res.UnitPath)
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return nil
}

// runRun launches a one-off VM from an image without requiring the
// caller to write a compose file. It is the holos analogue of
// `docker run`: the user names an image, optionally hangs flags off
// it for ports/volumes/resources/etc, and gets a running VM.
//
// Implementation: we synthesise a single-service compose.File, persist
// it to state_dir/runs/<auto-name>/holos.yaml, then load it through
// the same loadProject + manager.Up path everything else uses. Going
// through the on-disk file (rather than constructing a Project in
// memory directly) keeps follow-up commands like `holos exec`,
// `holos console`, and `holos down` working — they all expect a
// compose file path, and now there is one.
//
// VMs are inherently detached; a "foreground" mode would just be
// `holos run ... && holos console <name>-0`, which is what the
// printed hint suggests.
func runRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	name := flags.String("name", "", "project name (default: derived from image with random suffix)")
	vcpu := flags.Int("vcpu", 0, "vCPU count (default 1)")
	memory := flags.String("memory", "", "memory size, e.g. \"512M\", \"2G\" (default 512M)")
	user := flags.String("user", "", "cloud-init user (default: ubuntu)")
	dockerfile := flags.String("dockerfile", "", "use a Dockerfile to provision the VM (image arg becomes optional)")
	uefi := flags.Bool("uefi", false, "boot via OVMF (auto-enabled when --device is set)")
	detach := flags.Bool("detach", true, "start in background (kept for symmetry; foreground is not supported)")
	var ports, volumes, devices, packages, runcmd stringList
	flags.Var(&ports, "p", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&ports, "port", "publish a port HOST:GUEST (repeatable)")
	flags.Var(&volumes, "v", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&volumes, "volume", "bind mount HOSTPATH:GUESTPATH[:ro] (repeatable)")
	flags.Var(&devices, "device", "PCI address to pass through, e.g. 0000:01:00.0 (repeatable)")
	flags.Var(&packages, "pkg", "cloud-init package to install (repeatable)")
	flags.Var(&runcmd, "runcmd", "shell command to run on first boot (repeatable)")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	_ = detach // detached is the only mode; flag exists for surface-level docker parity

	// Trailing args after a `--` separator become extra runcmd entries
	// (`holos run alpine -- echo hello, world`). The stdlib flag parser
	// stops at the first non-flag positional, which means `--` is
	// preserved verbatim somewhere inside flags.Args() — we strip it
	// explicitly so it doesn't show up as a literal `--` in the
	// generated runcmd.
	positional := flags.Args()
	for i, a := range positional {
		if a == "--" {
			positional = append(positional[:i], positional[i+1:]...)
			break
		}
	}
	var image string
	var trailing []string
	if *dockerfile != "" {
		// Image is optional with --dockerfile; FROM line provides it.
		if len(positional) > 0 {
			image = positional[0]
			trailing = positional[1:]
		}
	} else {
		if len(positional) == 0 {
			return errors.New("run requires an image (e.g. `holos run ubuntu:noble`)")
		}
		image = positional[0]
		trailing = positional[1:]
	}
	if len(trailing) > 0 {
		runcmd = append(runcmd, strings.Join(trailing, " "))
	}

	memMB := 0
	if *memory != "" {
		mb, err := parseMemoryMB(*memory)
		if err != nil {
			return err
		}
		memMB = mb
	}

	devList := make([]compose.ComposeDevice, len(devices))
	for i, d := range devices {
		devList[i] = compose.ComposeDevice{PCI: d}
	}

	projectName := *name
	if projectName == "" {
		projectName = generateRunName(image, *dockerfile)
	}
	if !runNamePattern.MatchString(projectName) {
		return fmt.Errorf("project name %q must be a DNS label (lowercase letters, digits, hyphens)", projectName)
	}

	// We always emit one service called "vm" so instance names are
	// predictable: <project>'s lone instance is always "vm-0".
	const serviceName = "vm"

	svc := compose.Service{
		Image:      image,
		Dockerfile: *dockerfile,
		VM: compose.VM{
			VCPU:     *vcpu,
			MemoryMB: memMB,
			UEFI:     *uefi,
		},
		Ports:   ports,
		Volumes: volumes,
		Devices: devList,
		CloudInit: compose.CloudInit{
			User:     *user,
			Packages: packages,
			RunCmd:   runcmd,
		},
	}

	file := compose.File{
		Name:     projectName,
		Services: map[string]compose.Service{serviceName: svc},
	}

	// Persist the synthesised compose file so subsequent commands can
	// pick it up via -f. runs/ is sibling to projects/ and instances/
	// inside the state dir; nothing else writes there so collision
	// with other holos features is impossible.
	runDir := filepath.Join(*stateDir, "runs", projectName)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	composePath := filepath.Join(runDir, "holos.yaml")
	yamlBytes, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}
	if err := os.WriteFile(composePath, yamlBytes, 0o644); err != nil {
		return fmt.Errorf("write compose: %w", err)
	}

	project, err := loadProject(composePath, *stateDir)
	if err != nil {
		// Surface the synthesised yaml in the error so the user can
		// see exactly what we tried to launch when validation fails
		// (bad memory unit, malformed port spec, etc).
		return fmt.Errorf("synthesise project (see %s):\n%w", composePath, err)
	}

	manager := runtime.NewManager(*stateDir)
	record, err := manager.Up(project)
	if err != nil {
		return err
	}

	printProjectStatus(record)
	fmt.Printf("compose file: %s\n", composePath)
	fmt.Println()
	fmt.Println("next steps:")
	fmt.Printf("  holos exec -f %s vm-0\n", composePath)
	fmt.Printf("  holos console -f %s vm-0\n", composePath)
	fmt.Printf("  holos down %s\n", projectName)
	return nil
}

// stringList is a flag.Value that accepts a flag multiple times,
// accumulating each occurrence into a slice. Used for -p/-v/--device/
// --pkg/--runcmd in `holos run`.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// runNamePattern matches the same DNS-label rule compose uses for
// project and service names. We pre-validate here so the error is
// pinned to --name rather than appearing as a generic compose error.
var runNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var runNameSanitiser = regexp.MustCompile(`[^a-z0-9-]+`)

// generateRunName derives a deterministic-prefix + random-suffix name
// from an image reference, mirroring how docker auto-names containers
// when --name is omitted. The suffix means repeated `holos run alpine`
// invocations don't collide.
//
//	ubuntu:noble       -> ubuntu-noble-3f2a1c
//	./my-image.qcow2   -> my-image-qcow2-9d40a2
//	(--dockerfile)     -> dockerfile-7e1b04
func generateRunName(image, dockerfilePath string) string {
	base := image
	if base == "" {
		base = "dockerfile"
	}
	// Strip directory prefix from local paths so we get "my-image"
	// rather than "var-lib-libvirt-images-my-image".
	base = filepath.Base(base)
	// Drop a trailing ".qcow2"/".raw"/".img" extension if present.
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		base = base[:dot]
	}
	base = strings.ToLower(base)
	base = runNameSanitiser.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "vm"
	}
	// Reserve room for "-XXXXXX" suffix within the 63-char limit.
	const suffixLen = 7 // hyphen + 6 hex chars
	if len(base) > 63-suffixLen {
		base = base[:63-suffixLen]
		base = strings.TrimRight(base, "-")
	}
	suffix := randHex(3)
	_ = dockerfilePath // intentionally unused; suffix is the uniqueness guarantee
	return base + "-" + suffix
}

// randHex returns 2*n lowercase hex chars from crypto/rand. Falls back
// to a stringified nanosecond stamp on the (essentially impossible)
// case of crypto/rand failing, so we never panic out of name
// generation in user-facing flow.
func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(int64(os.Getpid()), 16)
	}
	return hex.EncodeToString(buf)
}

// parseMemoryMB accepts docker-style memory sizes ("512M", "2G", "1024")
// and returns a value in MiB suitable for compose.VM.MemoryMB. Bare
// integers are treated as megabytes, matching qemu's `-m` convention.
func parseMemoryMB(raw string) (int, error) {
	s := strings.TrimSpace(strings.ToUpper(raw))
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	multiplierMB := 1.0
	last := s[len(s)-1]
	switch last {
	case 'B':
		// allow "512MB", "2GB" — strip the B, look at the actual unit
		if len(s) < 2 {
			return 0, fmt.Errorf("invalid memory %q", raw)
		}
		s = s[:len(s)-1]
		last = s[len(s)-1]
		fallthrough
	case 'K', 'M', 'G', 'T':
		switch last {
		case 'K':
			multiplierMB = 1.0 / 1024.0
		case 'M':
			multiplierMB = 1
		case 'G':
			multiplierMB = 1024
		case 'T':
			multiplierMB = 1024 * 1024
		}
		s = s[:len(s)-1]
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q: %w", raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("memory %q must be positive", raw)
	}
	mb := int(value * multiplierMB)
	if mb < 1 {
		return 0, fmt.Errorf("memory %q rounds to less than 1 MB", raw)
	}
	return mb, nil
}

// runVersion prints the build metadata. When the binary was produced
// by goreleaser the values come from -ldflags injection; for a plain
// `go build` we recover commit + dirty flag from the runtime build
// info so users still see something useful.
func runVersion(args []string) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	short := flags.Bool("short", false, "print only the version string")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	v, c, d := version, commit, date
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if c == "none" && s.Value != "" {
					c = s.Value
				}
			case "vcs.time":
				if d == "unknown" && s.Value != "" {
					d = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" && c != "none" && !strings.HasSuffix(c, "-dirty") {
					c += "-dirty"
				}
			}
		}
	}

	if *short {
		fmt.Println(v)
		return nil
	}
	fmt.Printf("holos %s\n", v)
	fmt.Printf("  commit: %s\n", c)
	fmt.Printf("  built:  %s\n", d)
	fmt.Printf("  go:     %s\n", goruntime.Version())
	fmt.Printf("  os/arch: %s/%s\n", goruntime.GOOS, goruntime.GOARCH)
	return nil
}

// runImport translates one or more libvirt-defined VMs into a holos
// compose file. The mapping is intentionally lossy — fields holos has
// no concept of (bridged networks, secondary disks, USB passthrough)
// surface as warnings on stderr so the operator knows what to revisit
// before `holos up`. Output goes to stdout by default so it composes
// with shell redirection; -o writes to a path instead.
//
// Sources:
//
//	holos import vm1 vm2 ...        fetch via `virsh dumpxml`
//	holos import --all              every domain `virsh list --all` knows
//	holos import --xml domain.xml   read XML directly (no virsh needed)
func runImport(args []string) error {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	output := flags.String("o", "", "output file (default stdout; '-' is stdout)")
	projectName := flags.String("project", "", "project name (defaults to first imported domain)")
	fromXML := flags.String("xml", "", "read libvirt XML from a file instead of invoking virsh")
	connectURI := flags.String("connect", "", "libvirt connection URI passed as `virsh -c <uri>`")
	all := flags.Bool("all", false, "import every domain returned by `virsh list --all`")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	file := compose.File{Services: map[string]compose.Service{}}
	var allWarnings []string
	var order []string

	addDomain := func(label string, data []byte) error {
		name, svc, warns, err := virtimport.Convert(data)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if _, exists := file.Services[name]; exists {
			return fmt.Errorf("%s: service name %q already imported (rename the source domain)", label, name)
		}
		file.Services[name] = svc
		order = append(order, name)
		for _, w := range warns {
			allWarnings = append(allWarnings, fmt.Sprintf("%s: %s", name, w))
		}
		return nil
	}

	switch {
	case *fromXML != "":
		if flags.NArg() > 0 || *all {
			return errors.New("--xml cannot be combined with domain names or --all")
		}
		data, err := os.ReadFile(*fromXML)
		if err != nil {
			return fmt.Errorf("read xml: %w", err)
		}
		if err := addDomain(filepath.Base(*fromXML), data); err != nil {
			return err
		}
	default:
		v := virtimport.Virsh{URI: *connectURI}
		var domains []string
		switch {
		case *all && flags.NArg() > 0:
			return errors.New("--all cannot be combined with explicit domain names")
		case *all:
			list, err := v.ListDomains()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				return errors.New("virsh list --all returned no domains")
			}
			domains = list
		case flags.NArg() > 0:
			domains = flags.Args()
		default:
			return errors.New("import requires a domain name, --all, or --xml <file>")
		}
		for _, dom := range domains {
			data, err := v.DumpXML(dom)
			if err != nil {
				return err
			}
			if err := addDomain(dom, data); err != nil {
				return err
			}
		}
	}

	switch {
	case *projectName != "":
		file.Name = *projectName
	case len(order) > 0:
		file.Name = order[0]
	default:
		file.Name = "imported"
	}

	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal compose: %w", err)
	}

	for _, w := range allWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	if *output == "" || *output == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d service(s))\n", *output, len(file.Services))
	return nil
}

// loadProjectWithPath is loadProject plus the absolute path of the
// compose file it found — installers need the path to embed in the
// unit's ExecStart=.
func loadProjectWithPath(filePath, stateDir string) (*compose.Project, string, error) {
	if filePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", fmt.Errorf("get working directory: %w", err)
		}
		found, err := compose.FindFile(cwd)
		if err != nil {
			return nil, "", err
		}
		filePath = found
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve compose path: %w", err)
	}

	file, err := compose.Load(abs)
	if err != nil {
		return nil, "", err
	}
	project, err := file.Resolve(filepath.Dir(abs), stateDir)
	if err != nil {
		return nil, "", err
	}
	return project, abs, nil
}

// loadProject finds, loads, and resolves a compose file.
func loadProject(filePath string, stateDir string) (*compose.Project, error) {
	if filePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		found, err := compose.FindFile(cwd)
		if err != nil {
			return nil, err
		}
		filePath = found
	}

	file, err := compose.Load(filePath)
	if err != nil {
		return nil, err
	}

	baseDir := filepath.Dir(filePath)
	abs, err := filepath.Abs(baseDir)
	if err == nil {
		baseDir = abs
	}

	return file.Resolve(baseDir, stateDir)
}

func printProjectStatus(record *runtime.ProjectRecord) {
	fmt.Printf("project: %s\n\n", record.Name)

	for _, svc := range record.Services {
		fmt.Printf("service: %s (%d/%d running)\n", svc.Name, svc.RunningCount(), svc.DesiredReplicas)

		writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(writer, "  INSTANCE\tSTATUS\tPID\tPORTS\tLOG")
		for _, inst := range svc.Instances {
			fmt.Fprintf(writer, "  %s\t%s\t%d\t%s\t%s\n",
				inst.Name,
				inst.Status,
				inst.PID,
				inst.PortSummary(),
				inst.LogPath,
			)
		}
		_ = writer.Flush()
		fmt.Println()
	}
}

func printLogTail(path string, lines int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (cannot read log: %v)\n", err)
		return
	}

	allLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	for _, line := range allLines[start:] {
		fmt.Println(line)
	}
}

func servicePorts(svc runtime.ServiceRecord) string {
	if len(svc.Instances) == 0 {
		return "-"
	}
	return svc.Instances[0].PortSummary()
}

func printJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func usage() {
	fmt.Fprintf(os.Stderr, `holos - docker compose for KVM

Usage:
  holos up [-f holos.yaml]             start all services
  holos run [flags] <image> [-- cmd...]
                                       launch a one-off VM from an image (no compose file)
  holos down [-f holos.yaml]           stop and remove all services
  holos ps                             list running projects
  holos start [-f holos.yaml] [svc]    start a stopped service or all services
  holos stop [-f holos.yaml] [svc]     stop a service or all services
  holos console [-f holos.yaml] <inst> attach serial console to an instance
  holos exec [-f holos.yaml] <inst> [cmd...]  ssh into an instance (project's generated key)
  holos logs [-f holos.yaml] <svc>     show service logs
  holos validate [-f holos.yaml]       validate compose file
  holos pull <image>                   pull a cloud image (e.g. alpine, ubuntu:noble)
  holos images                         list available images
  holos devices [--gpu]                list PCI devices and IOMMU groups
  holos install [-f holos.yaml] [--system] [--enable]
                                       emit a systemd unit so the project comes back up on reboot
  holos uninstall [-f holos.yaml] [--system]
                                       remove the systemd unit written by 'holos install'
  holos import [vm...] [--all] [--xml file] [--connect uri] [-o file]
                                       convert virsh-defined VMs into a holos.yaml
  holos version [--short]              print build version, commit, and platform
`)
}
