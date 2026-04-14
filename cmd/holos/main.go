package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/rich/holosteric/internal/compose"
	"github.com/rich/holosteric/internal/images"
	"github.com/rich/holosteric/internal/runtime"
	"github.com/rich/holosteric/internal/vfio"
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
	case "down":
		return runDown(args[1:])
	case "ps":
		return runPS(args[1:])
	case "stop":
		return runStop(args[1:])
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

	content := string(data)
	allLines := splitLines(content)
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}
	for _, line := range allLines[start:] {
		fmt.Println(line)
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	for len(s) > 0 {
		idx := 0
		for idx < len(s) && s[idx] != '\n' {
			idx++
		}
		lines = append(lines, s[:idx])
		if idx < len(s) {
			s = s[idx+1:]
		} else {
			break
		}
	}
	return lines
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
  holos up [-f holos.yaml]         start all services
  holos down [-f holos.yaml]       stop and remove all services
  holos ps                         list running projects
  holos stop [-f holos.yaml] [svc] stop a service or all services
  holos logs [-f holos.yaml] <svc> show service logs
  holos validate [-f holos.yaml]   validate compose file
  holos pull <image>               pull a cloud image (e.g. alpine, ubuntu:noble)
  holos images                     list available images
  holos devices [--gpu]            list PCI devices and IOMMU groups
`)
}
