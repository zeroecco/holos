package main

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/zeroecco/holos/internal/runtime"
)

const (
	doctorQEMUSystemBinary = "qemu-system-x86_64"
	doctorQEMUSystemEnv    = "HOLOS_QEMU_SYSTEM"
	doctorQEMUImgBinary    = "qemu-img"
	doctorQEMUImgEnv       = "HOLOS_QEMU_IMG"
	sshClientBinary        = "ssh"
	doctorOVMFCodeEnv      = "HOLOS_OVMF_CODE"
	doctorOVMFVarsEnv      = "HOLOS_OVMF_VARS"
	doctorVersionFlag      = "--version"
	doctorHelpFlag         = "--help"
	doctorSSHVersionFlag   = "-V"
	doctorXorrisoVersion   = "-version"
	doctorCloudLocalDS     = "cloud-localds"
	doctorGenisoimage      = "genisoimage"
	doctorMkisofs          = "mkisofs"
	doctorXorriso          = "xorriso"
	doctorHostOSCheckName  = "host os"
	doctorKVMCheckName     = "/dev/kvm"
	doctorKVMPath          = "/dev/kvm"
	doctorOVMFCheckName    = "OVMF firmware"
	doctorStateDirName     = "state dir"
	doctorStateDirPerm     = os.FileMode(0o700)
)

func buildDoctorReport(stateDir string) doctorReport {
	report := doctorReport{
		OS:       goruntime.GOOS,
		Arch:     goruntime.GOARCH,
		StateDir: stateDir,
	}

	report.Checks = append(report.Checks, checkHostOS())
	report.Checks = append(report.Checks, checkKVM())
	report.Checks = append(report.Checks, checkCommand(doctorQEMUSystemBinary, doctorQEMUSystemEnv, []string{doctorVersionFlag}, "required to launch VMs"))
	report.Checks = append(report.Checks, checkCommand(doctorQEMUImgBinary, doctorQEMUImgEnv, []string{doctorVersionFlag}, "required to create overlays and volumes"))
	report.Checks = append(report.Checks, checkAnyCommand("cloud-init seed builder", []doctorCommand{
		{name: doctorCloudLocalDS, args: []string{doctorHelpFlag}},
		{name: doctorGenisoimage, args: []string{doctorVersionFlag}},
		{name: doctorMkisofs, args: []string{doctorVersionFlag}},
		{name: doctorXorriso, args: []string{doctorXorrisoVersion}},
	}, "required to create NoCloud seed media"))
	report.Checks = append(report.Checks, checkCommand(sshClientBinary, "", []string{doctorSSHVersionFlag}, "required for holos exec and healthchecks"))
	report.Checks = append(report.Checks, checkOVMF())
	report.Checks = append(report.Checks, checkStateDir(stateDir))
	return report
}

func checkHostOS() doctorCheck {
	if goruntime.GOOS == "linux" {
		return doctorCheck{Name: doctorHostOSCheckName, Status: doctorStatusOK, Message: "Linux host can run KVM workloads"}
	}
	return doctorCheck{Name: doctorHostOSCheckName, Status: doctorStatusWarn, Message: "only offline commands work on " + goruntime.GOOS + "; up/run require Linux + KVM"}
}

func checkKVM() doctorCheck {
	if goruntime.GOOS != "linux" {
		return doctorCheck{Name: doctorKVMCheckName, Status: doctorStatusWarn, Message: "KVM is Linux-only; run workloads on a Linux host"}
	}
	f, err := os.OpenFile(doctorKVMPath, os.O_RDWR, 0)
	if err != nil {
		return doctorCheck{Name: doctorKVMCheckName, Status: doctorStatusFail, Message: "cannot open " + doctorKVMPath + " read-write; enable virtualization, load kvm/kvm-intel or kvm-amd, and check group permissions: " + err.Error()}
	}
	_ = f.Close()
	return doctorCheck{Name: doctorKVMCheckName, Status: doctorStatusOK, Message: "KVM device opens read-write"}
}

func checkOVMF() doctorCheck {
	firmware, err := runtime.ResolveOVMFFirmware()
	if err != nil {
		if doctorOVMFEnvOverrideConfigured(os.Getenv(doctorOVMFCodeEnv), os.Getenv(doctorOVMFVarsEnv)) {
			return doctorCheck{Name: doctorOVMFCheckName, Status: doctorStatusFail, Message: err.Error()}
		}
		return doctorCheck{Name: doctorOVMFCheckName, Status: doctorStatusWarn, Message: err.Error()}
	}
	return doctorCheck{Name: doctorOVMFCheckName, Status: doctorStatusOK, Message: doctorOVMFSuccessMessage(firmware)}
}

func doctorOVMFEnvOverrideConfigured(codeEnv, varsEnv string) bool {
	return codeEnv != "" || varsEnv != ""
}

func doctorOVMFSuccessMessage(firmware runtime.OVMFFirmware) string {
	return fmt.Sprintf("CODE=%s VARS=%s", firmware.CodePath, firmware.VarsTemplatePath)
}

func checkStateDir(stateDir string) doctorCheck {
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return doctorCheck{Name: doctorStateDirName, Status: doctorStatusFail, Message: "cannot resolve state dir: " + err.Error()}
	}
	if err := os.MkdirAll(abs, doctorStateDirPerm); err != nil {
		return doctorCheck{Name: doctorStateDirName, Status: doctorStatusFail, Message: "cannot create " + abs + ": " + err.Error()}
	}
	tmp, err := os.CreateTemp(abs, ".doctor-*")
	if err != nil {
		return doctorCheck{Name: doctorStateDirName, Status: doctorStatusFail, Message: "cannot write to " + abs + ": " + err.Error()}
	}
	name := tmp.Name()
	closeErr := tmp.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return doctorCheck{Name: doctorStateDirName, Status: doctorStatusFail, Message: "cannot close test file in " + abs + ": " + closeErr.Error()}
	}
	if removeErr != nil {
		return doctorCheck{Name: doctorStateDirName, Status: doctorStatusWarn, Message: "wrote test file but could not remove it: " + removeErr.Error()}
	}
	return doctorCheck{Name: doctorStateDirName, Status: doctorStatusOK, Message: abs + " is writable"}
}

func doctorHasFailure(report doctorReport) bool {
	for _, check := range report.Checks {
		if check.Status == doctorStatusFail {
			return true
		}
	}
	return false
}
