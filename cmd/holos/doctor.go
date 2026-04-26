package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/zeroecco/holos/internal/runtime"
)

type doctorReport struct {
	OS       string        `json:"os"`
	Arch     string        `json:"arch"`
	StateDir string        `json:"state_dir"`
	Checks   []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func runDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	stateDir := flags.String("state-dir", runtime.DefaultStateDir(), "state directory")
	jsonOut := flags.Bool("json", false, "emit JSON")
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}

	report := buildDoctorReport(*stateDir)
	if *jsonOut {
		if err := printJSON(report); err != nil {
			return err
		}
	} else {
		printDoctorReport(report)
	}

	if doctorHasFailure(report) {
		return errors.New("doctor found failed checks")
	}
	return nil
}

func buildDoctorReport(stateDir string) doctorReport {
	report := doctorReport{
		OS:       goruntime.GOOS,
		Arch:     goruntime.GOARCH,
		StateDir: stateDir,
	}

	report.Checks = append(report.Checks, checkHostOS())
	report.Checks = append(report.Checks, checkKVM())
	report.Checks = append(report.Checks, checkCommand("qemu-system-x86_64", "HOLOS_QEMU_SYSTEM", []string{"--version"}, "required to launch VMs"))
	report.Checks = append(report.Checks, checkCommand("qemu-img", "HOLOS_QEMU_IMG", []string{"--version"}, "required to create overlays and volumes"))
	report.Checks = append(report.Checks, checkAnyCommand("cloud-init seed builder", []doctorCommand{
		{name: "cloud-localds", args: []string{"--help"}},
		{name: "genisoimage", args: []string{"--version"}},
		{name: "mkisofs", args: []string{"--version"}},
		{name: "xorriso", args: []string{"-version"}},
	}, "required to create NoCloud seed media"))
	report.Checks = append(report.Checks, checkCommand("ssh", "", []string{"-V"}, "required for holos exec and healthchecks"))
	report.Checks = append(report.Checks, checkOVMF())
	report.Checks = append(report.Checks, checkStateDir(stateDir))
	return report
}

func checkHostOS() doctorCheck {
	if goruntime.GOOS == "linux" {
		return doctorCheck{Name: "host os", Status: "ok", Message: "Linux host can run KVM workloads"}
	}
	return doctorCheck{Name: "host os", Status: "warn", Message: "only offline commands work on " + goruntime.GOOS + "; up/run require Linux + KVM"}
}

func checkKVM() doctorCheck {
	if goruntime.GOOS != "linux" {
		return doctorCheck{Name: "/dev/kvm", Status: "warn", Message: "KVM is Linux-only; run workloads on a Linux host"}
	}
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return doctorCheck{Name: "/dev/kvm", Status: "fail", Message: "cannot open /dev/kvm read-write; enable virtualization, load kvm/kvm-intel or kvm-amd, and check group permissions: " + err.Error()}
	}
	_ = f.Close()
	return doctorCheck{Name: "/dev/kvm", Status: "ok", Message: "KVM device opens read-write"}
}

type doctorCommand struct {
	name string
	args []string
}

func checkCommand(name, envVar string, args []string, purpose string) doctorCheck {
	path := ""
	if override := os.Getenv(envVar); override != "" {
		if err := checkExecutable(override); err != nil {
			return doctorCheck{Name: name, Status: "fail", Message: fmt.Sprintf("%s points to %s, but it is not executable: %v", envVar, override, err)}
		}
		path = override
	} else if found, err := exec.LookPath(name); err == nil {
		path = found
	} else {
		if envVar != "" {
			return doctorCheck{Name: name, Status: "fail", Message: purpose + "; install it or set " + envVar}
		}
		return doctorCheck{Name: name, Status: "fail", Message: purpose + "; install " + name}
	}

	out, err := runDoctorProbe(path, args)
	if err != nil {
		return doctorCheck{Name: name, Status: "fail", Message: fmt.Sprintf("%s found at %s but probe failed: %v", name, path, err)}
	}
	if out != "" {
		return doctorCheck{Name: name, Status: "ok", Message: fmt.Sprintf("%s (%s)", path, out)}
	}
	return doctorCheck{Name: name, Status: "ok", Message: path}
}

func checkAnyCommand(label string, commands []doctorCommand, purpose string) doctorCheck {
	var failures []string
	for _, candidate := range commands {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			failures = append(failures, candidate.name+": not found")
			continue
		}
		out, err := runDoctorProbe(path, candidate.args)
		if err != nil {
			failures = append(failures, candidate.name+": "+err.Error())
			continue
		}
		if out != "" {
			return doctorCheck{Name: label, Status: "ok", Message: fmt.Sprintf("%s at %s (%s)", candidate.name, path, out)}
		}
		return doctorCheck{Name: label, Status: "ok", Message: fmt.Sprintf("%s at %s", candidate.name, path)}
	}
	var names []string
	for _, candidate := range commands {
		names = append(names, candidate.name)
	}
	detail := purpose + "; install one of " + joinNames(names)
	if len(failures) > 0 {
		detail += " (" + strings.Join(failures, "; ") + ")"
	}
	return doctorCheck{Name: label, Status: "fail", Message: detail}
}

func checkOVMF() doctorCheck {
	firmware, err := runtime.ResolveOVMFFirmware()
	if err != nil {
		if os.Getenv("HOLOS_OVMF_CODE") != "" || os.Getenv("HOLOS_OVMF_VARS") != "" {
			return doctorCheck{Name: "OVMF firmware", Status: "fail", Message: err.Error()}
		}
		return doctorCheck{Name: "OVMF firmware", Status: "warn", Message: err.Error()}
	}
	return doctorCheck{Name: "OVMF firmware", Status: "ok", Message: fmt.Sprintf("CODE=%s VARS=%s", firmware.CodePath, firmware.VarsTemplatePath)}
}

func checkStateDir(stateDir string) doctorCheck {
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return doctorCheck{Name: "state dir", Status: "fail", Message: "cannot resolve state dir: " + err.Error()}
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return doctorCheck{Name: "state dir", Status: "fail", Message: "cannot create " + abs + ": " + err.Error()}
	}
	tmp, err := os.CreateTemp(abs, ".doctor-*")
	if err != nil {
		return doctorCheck{Name: "state dir", Status: "fail", Message: "cannot write to " + abs + ": " + err.Error()}
	}
	name := tmp.Name()
	closeErr := tmp.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return doctorCheck{Name: "state dir", Status: "fail", Message: "cannot close test file in " + abs + ": " + closeErr.Error()}
	}
	if removeErr != nil {
		return doctorCheck{Name: "state dir", Status: "warn", Message: "wrote test file but could not remove it: " + removeErr.Error()}
	}
	return doctorCheck{Name: "state dir", Status: "ok", Message: abs + " is writable"}
}

func printDoctorReport(report doctorReport) {
	fmt.Printf("holos doctor (%s/%s)\n", report.OS, report.Arch)
	fmt.Printf("state dir: %s\n\n", report.StateDir)

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "CHECK\tSTATUS\tDETAIL")
	for _, check := range report.Checks {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", check.Name, check.Status, check.Message)
	}
	_ = writer.Flush()
}

func doctorHasFailure(report doctorReport) bool {
	for _, check := range report.Checks {
		if check.Status == "fail" {
			return true
		}
	}
	return false
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	out := names[0]
	for _, name := range names[1:] {
		out += ", " + name
	}
	return out
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("execute bit is not set")
	}
	return nil
}

func runDoctorProbe(path string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("probe timed out")
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return "", err
	}
	return firstLine(strings.TrimSpace(string(out))), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
