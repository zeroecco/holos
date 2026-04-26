package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"text/tabwriter"

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
	report.Checks = append(report.Checks, checkBinary("qemu-system-x86_64", "HOLOS_QEMU_SYSTEM", "required to launch VMs"))
	report.Checks = append(report.Checks, checkBinary("qemu-img", "HOLOS_QEMU_IMG", "required to create overlays and volumes"))
	report.Checks = append(report.Checks, checkAnyBinary("cloud-init seed builder", []string{"cloud-localds", "genisoimage", "mkisofs", "xorriso"}, "required to create NoCloud seed media"))
	report.Checks = append(report.Checks, checkAnyBinary("ssh", []string{"ssh"}, "required for holos exec and healthchecks"))
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
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return doctorCheck{Name: "/dev/kvm", Status: "fail", Message: "missing /dev/kvm; enable virtualization and load kvm/kvm-intel or kvm-amd"}
	}
	return doctorCheck{Name: "/dev/kvm", Status: "ok", Message: "KVM device exists"}
}

func checkBinary(name, envVar, purpose string) doctorCheck {
	if override := os.Getenv(envVar); override != "" {
		if _, err := os.Stat(override); err != nil {
			return doctorCheck{Name: name, Status: "fail", Message: fmt.Sprintf("%s points to %s, but it is not readable: %v", envVar, override, err)}
		}
		return doctorCheck{Name: name, Status: "ok", Message: fmt.Sprintf("using %s=%s", envVar, override)}
	}
	if path, err := exec.LookPath(name); err == nil {
		return doctorCheck{Name: name, Status: "ok", Message: path}
	}
	return doctorCheck{Name: name, Status: "fail", Message: purpose + "; install it or set " + envVar}
}

func checkAnyBinary(label string, names []string, purpose string) doctorCheck {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return doctorCheck{Name: label, Status: "ok", Message: fmt.Sprintf("%s at %s", name, path)}
		}
	}
	return doctorCheck{Name: label, Status: "fail", Message: purpose + "; install one of " + joinNames(names)}
}

func checkOVMF() doctorCheck {
	if code := os.Getenv("HOLOS_OVMF_CODE"); code != "" {
		if _, err := os.Stat(code); err != nil {
			return doctorCheck{Name: "OVMF firmware", Status: "warn", Message: fmt.Sprintf("HOLOS_OVMF_CODE points to %s, but it is not readable: %v", code, err)}
		}
		return doctorCheck{Name: "OVMF firmware", Status: "ok", Message: "using HOLOS_OVMF_CODE=" + code}
	}
	for _, path := range ovmfCodeCandidates {
		if _, err := os.Stat(path); err == nil {
			return doctorCheck{Name: "OVMF firmware", Status: "ok", Message: path}
		}
	}
	return doctorCheck{Name: "OVMF firmware", Status: "warn", Message: "not found; install ovmf/edk2-ovmf or set HOLOS_OVMF_CODE before using UEFI or PCI passthrough"}
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

var ovmfCodeCandidates = []string{
	"/usr/share/OVMF/OVMF_CODE_4M.fd",
	"/usr/share/OVMF/OVMF_CODE.fd",
	"/usr/share/edk2/ovmf/OVMF_CODE.fd",
	"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd",
	"/usr/share/qemu/OVMF_CODE.fd",
}
