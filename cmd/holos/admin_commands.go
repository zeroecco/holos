package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/images"
	"github.com/zeroecco/holos/internal/runtime"
	"github.com/zeroecco/holos/internal/systemd"
	"github.com/zeroecco/holos/internal/vfio"
	"github.com/zeroecco/holos/internal/virtimport"
	"gopkg.in/yaml.v3"
)

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
	fmt.Printf("order: %v\n\n", project.ServiceOrder)

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

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	all := flags.Bool("all", false, "verify every cached registry image with checksum metadata")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	refs := flags.Args()
	if *all {
		if len(refs) != 0 {
			return errors.New("verify --all does not accept image arguments")
		}
		for _, img := range images.ListAvailable() {
			refs = append(refs, img.Name+":"+img.Tag)
		}
	} else if len(refs) == 0 {
		return errors.New("verify requires an image name, local path, or --all")
	}

	cacheDir := images.DefaultCacheDir(*stateDir)
	for _, ref := range refs {
		res, err := images.Verify(ref, cacheDir)
		if err != nil {
			if *all && os.IsNotExist(err) {
				fmt.Printf("%s: skipped (not cached)\n", ref)
				continue
			}
			return fmt.Errorf("verify %s: %w", ref, err)
		}
		if res.Skipped {
			fmt.Printf("%s: skipped (%s)\n", ref, res.Reason)
			continue
		}
		fmt.Printf("%s: verified %s:%s %s\n", ref, res.Algorithm, res.Hash[:16], res.Path)
	}
	return nil
}

func runImages(_ []string) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTAG\tFORMAT\tOS\tVERIFY")
	for _, img := range images.ListAvailable() {
		name := img.Name
		if img.Default {
			name += " *"
		}
		verify := "-"
		switch {
		case img.SHA256 != "" || img.SHA256URL != "":
			verify = "sha256"
		case img.SHA512 != "" || img.SHA512URL != "":
			verify = "sha512"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", name, img.Tag, img.Format, img.OSFamily, verify)
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

	stateDirExplicit := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "state-dir" {
			stateDirExplicit = true
		}
	})
	if *system && *runAs != "" && !stateDirExplicit {
		return fmt.Errorf(
			"install --system --user %s requires --state-dir pointing at a directory %[1]s can read and write; "+
				"holos locks the state tree to 0700 so running as %[1]s would otherwise fail at start",
			*runAs)
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
	} else if err := compose.ValidateName(projectName); err != nil {
		return fmt.Errorf("invalid --name: %w", err)
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
  holos up [-f holos.yaml] [--lock-timeout 5m|--no-wait]
                                       start all services
  holos run [flags] <image> [-- cmd...]
                                       launch a one-off VM from an image (no compose file)
  holos down [-f holos.yaml]           stop and remove all services
  holos ps [-f holos.yaml]             list running projects (-f narrows to one)
  holos start [-f holos.yaml] [svc]    start a stopped service or all services
  holos stop [-f holos.yaml] [svc]     stop a service or all services
  holos console [-f holos.yaml] <inst> attach serial console to an instance
  holos exec [-f holos.yaml] <inst> [cmd...]  ssh into an instance (project's generated key)
  holos logs [-f holos.yaml] <svc|inst>  show logs for a service (all replicas) or one instance
  holos validate [-f holos.yaml]       validate compose file
  holos pull <image>                   pull a cloud image (e.g. alpine, ubuntu:noble)
  holos verify <image>|--all           verify cached image checksums
  holos images                         list available images
  holos devices [--gpu]                list PCI devices and IOMMU groups
  holos doctor [--json]                check host dependencies and state dir access
  holos install [-f holos.yaml] [--system] [--enable]
                                       emit a systemd unit so the project comes back up on reboot
  holos uninstall [-f holos.yaml] [--system]
                                       remove the systemd unit written by 'holos install'
  holos import [vm...] [--all] [--xml file] [--connect uri] [-o file]
                                       convert virsh-defined VMs into a holos.yaml
  holos version [--short]              print build version, commit, and platform
`)
}
