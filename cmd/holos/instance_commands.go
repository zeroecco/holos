package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/console"
	"github.com/zeroecco/holos/internal/runtime"
)

func runLogs(args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	lines := flags.Int("n", 50, "number of lines")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return errors.New("logs requires a project name (e.g. \"my-stack\") or a service/instance (e.g. \"vm\", \"vm-0\")")
	}

	manager := runtime.NewManager(*stateDir)
	var (
		record *runtime.ProjectRecord
		filter string
	)
	if *filePath == "" {
		if r, ok := lookupProjectRecord(manager, flags.Arg(0)); ok {
			record = r
			if flags.NArg() >= 2 {
				filter = flags.Arg(1)
			}
		}
	}
	if record == nil {
		if flags.NArg() != 1 {
			return errors.New("logs <project> [<service|instance>]  OR  logs [-f file] <service|instance>")
		}
		project, err := loadProject(*filePath, *stateDir)
		if err != nil {
			return err
		}
		record, err = manager.ProjectStatus(project.Name)
		if err != nil {
			return err
		}
		filter = flags.Arg(0)
	}

	var matches []runtime.InstanceRecord
	if filter == "" {
		for _, svc := range record.Services {
			matches = append(matches, svc.Instances...)
		}
	} else {
		matches = resolveLogTargets(record, filter)
		if len(matches) == 0 {
			return fmt.Errorf("no service or instance named %q in project %q", filter, record.Name)
		}
	}

	for _, inst := range matches {
		fmt.Printf("==> %s <==\n", inst.Name)
		printLogTail(inst.LogPath, *lines)
		fmt.Println()
	}
	return nil
}

func runExec(args []string) error {
	flags := flag.NewFlagSet("exec", flag.ContinueOnError)
	filePath := flags.String("f", "", "path to holos.yaml")
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	user := flags.String("u", "", "override login user (default: service's cloud-init user)")
	wait := flags.Duration("w", 60*time.Second, "wait up to this long for sshd to be ready (0 disables)")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return errors.New("exec requires a project name or an instance name (e.g. \"my-stack\" or \"web-0\")")
	}

	manager := runtime.NewManager(*stateDir)
	tgt, err := resolveInstanceTarget(manager, *filePath, *stateDir, flags.Args())
	if err != nil {
		return err
	}
	if tgt.Inst.Status != "running" {
		return fmt.Errorf("instance %q is %s", tgt.Inst.Name, tgt.Inst.Status)
	}
	if tgt.Inst.SSHPort == 0 {
		return fmt.Errorf("instance %q has no ssh port (created before exec support; recreate the stack)", tgt.Inst.Name)
	}

	loginUser := *user
	if loginUser == "" {
		loginUser = tgt.LoginUser
	}
	if loginUser == "" {
		loginUser = "ubuntu"
	}

	inst := tgt.Inst
	cmd := tgt.CmdArgs
	if *wait > 0 {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(inst.SSHPort))
		if !sshdReady(addr) {
			fmt.Fprintf(os.Stderr, "waiting up to %s for sshd on %s (cloud-init may still be regenerating host keys) ...\n", *wait, addr)
			if err := waitForSSHReady(addr, *wait); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v; attempting ssh anyway\n", err)
			}
		}
	}

	keyPath, err := manager.ProjectSSHKeyPath(tgt.ProjectName)
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
	if flags.NArg() < 1 {
		return errors.New("console requires a project name or an instance name (e.g. \"my-stack\" or \"web-0\")")
	}

	manager := runtime.NewManager(*stateDir)
	tgt, err := resolveInstanceTarget(manager, *filePath, *stateDir, flags.Args())
	if err != nil {
		return err
	}
	if tgt.Inst.Status != "running" {
		return fmt.Errorf("instance %q is %s", tgt.Inst.Name, tgt.Inst.Status)
	}
	if tgt.Inst.SerialPath == "" {
		return fmt.Errorf("instance %q has no serial console (created before console support)", tgt.Inst.Name)
	}
	return console.Attach(tgt.Inst.SerialPath)
}

type instanceTarget struct {
	Inst        runtime.InstanceRecord
	ProjectName string
	LoginUser   string
	CmdArgs     []string
}

func resolveInstanceTarget(manager *runtime.Manager, filePath, stateDir string, positional []string) (instanceTarget, error) {
	if len(positional) == 0 {
		return instanceTarget{}, errors.New("missing project or instance name")
	}
	if filePath == "" {
		if err := compose.ValidateName(positional[0]); err != nil {
			return instanceTarget{}, fmt.Errorf("invalid project name: %w", err)
		}
		if record, ok := lookupProjectRecord(manager, positional[0]); ok {
			tgt := instanceTarget{ProjectName: record.Name}
			if len(positional) >= 2 {
				if inst, svc, ok := findInstanceInRecord(record, positional[1]); ok {
					tgt.Inst = inst
					tgt.LoginUser = serviceLoginUser(svc, record, stateDir)
					tgt.CmdArgs = positional[2:]
					return tgt, nil
				}
			}
			inst, svc, ok := soleInstance(record)
			if !ok {
				missing := ""
				if len(positional) >= 2 {
					missing = fmt.Sprintf(" (no instance %q)", positional[1])
				}
				return instanceTarget{}, fmt.Errorf(
					"project %q has multiple instances%s; specify one (available: %s)",
					record.Name, missing, instanceList(record))
			}
			tgt.Inst = inst
			tgt.LoginUser = serviceLoginUser(svc, record, stateDir)
			tgt.CmdArgs = positional[1:]
			return tgt, nil
		}
	}

	project, err := loadProject(filePath, stateDir)
	if err != nil {
		return instanceTarget{}, err
	}
	inst, svcName, err := manager.FindInstance(project.Name, positional[0])
	if err != nil {
		return instanceTarget{}, err
	}
	tgt := instanceTarget{
		Inst:        inst,
		ProjectName: project.Name,
		CmdArgs:     positional[1:],
	}
	if svc, ok := project.Services[svcName]; ok && svc.CloudInit.User != "" {
		tgt.LoginUser = svc.CloudInit.User
	}
	return tgt, nil
}

func lookupProjectRecord(manager *runtime.Manager, name string) (*runtime.ProjectRecord, bool) {
	if name == "" {
		return nil, false
	}
	record, err := manager.ProjectStatus(name)
	if err != nil {
		return nil, false
	}
	return record, true
}

func findInstanceInRecord(record *runtime.ProjectRecord, instanceName string) (runtime.InstanceRecord, runtime.ServiceRecord, bool) {
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Name == instanceName {
				return inst, svc, true
			}
		}
	}
	return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
}

func soleInstance(record *runtime.ProjectRecord) (runtime.InstanceRecord, runtime.ServiceRecord, bool) {
	var (
		hitInst runtime.InstanceRecord
		hitSvc  runtime.ServiceRecord
		count   int
	)
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			count++
			if count > 1 {
				return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
			}
			hitInst = inst
			hitSvc = svc
		}
	}
	if count == 1 {
		return hitInst, hitSvc, true
	}
	return runtime.InstanceRecord{}, runtime.ServiceRecord{}, false
}

func instanceList(record *runtime.ProjectRecord) string {
	var names []string
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			names = append(names, inst.Name)
		}
	}
	return strings.Join(names, ", ")
}

func resolveLogTargets(record *runtime.ProjectRecord, name string) []runtime.InstanceRecord {
	for _, svc := range record.Services {
		if svc.Name == name {
			return svc.Instances
		}
	}
	for _, svc := range record.Services {
		for _, inst := range svc.Instances {
			if inst.Name == name {
				return []runtime.InstanceRecord{inst}
			}
		}
	}
	return nil
}

func serviceLoginUser(svc runtime.ServiceRecord, record *runtime.ProjectRecord, stateDir string) string {
	if svc.LoginUser != "" {
		return svc.LoginUser
	}
	if record != nil {
		for _, other := range record.Services {
			if other.LoginUser != "" {
				return other.LoginUser
			}
		}
		return lookupLoginUser(stateDir, record.Name)
	}
	return ""
}

func lookupLoginUser(stateDir, projectName string) string {
	composePath := filepath.Join(stateDir, "runs", projectName, "holos.yaml")
	file, err := compose.Load(composePath)
	if err != nil {
		return ""
	}
	project, err := file.Resolve(filepath.Dir(composePath), stateDir)
	if err != nil {
		return ""
	}
	for _, svc := range project.Services {
		if svc.CloudInit.User != "" {
			return svc.CloudInit.User
		}
	}
	return ""
}

func sshdReady(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	n, err := conn.Read(buf)
	if err != nil || n < 4 {
		return false
	}
	return string(buf) == "SSH-"
}

func waitForSSHReady(addr string, total time.Duration) error {
	deadline := time.Now().Add(total)
	delay := 200 * time.Millisecond
	for time.Now().Before(deadline) {
		if sshdReady(addr) {
			return nil
		}
		time.Sleep(delay)
		if delay < 2*time.Second {
			delay *= 2
		}
	}
	return fmt.Errorf("sshd on %s did not become ready within %s", addr, total)
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
