package runtime

import (
	"fmt"
	"os"
)

// OVMFFirmware is the resolved CODE/VARS firmware pair used for UEFI boots.
type OVMFFirmware struct {
	CodePath         string
	VarsTemplatePath string
}

const (
	ovmfCodeEnv  = "HOLOS_OVMF_CODE"
	ovmfVarsEnv  = "HOLOS_OVMF_VARS"
	ovmfVarsPerm = os.FileMode(0o644)
)

var ovmfCodePaths = []string{
	"/usr/share/OVMF/OVMF_CODE_4M.fd",
	"/usr/share/OVMF/OVMF_CODE.fd",
	"/usr/share/edk2/ovmf/OVMF_CODE.fd",
	"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd",
	"/usr/share/qemu/OVMF_CODE.fd",
}

var ovmfVarsPaths = []string{
	"/usr/share/OVMF/OVMF_VARS_4M.fd",
	"/usr/share/OVMF/OVMF_VARS.fd",
	"/usr/share/edk2/ovmf/OVMF_VARS.fd",
	"/usr/share/edk2-ovmf/x64/OVMF_VARS.fd",
	"/usr/share/qemu/OVMF_VARS.fd",
}

func (m *Manager) prepareUEFI(workDir string) (codePath, varsPath string, err error) {
	firmware, err := ResolveOVMFFirmware()
	if err != nil {
		return "", "", err
	}

	varsPath = newInstancePaths(workDir).ovmfVars
	if err := copyFile(firmware.VarsTemplatePath, varsPath); err != nil {
		return "", "", fmt.Errorf("copy OVMF_VARS: %w", err)
	}

	return firmware.CodePath, varsPath, nil
}

// ResolveOVMFFirmware locates a usable OVMF CODE/VARS template pair. If either
// environment override is set, both must be set; otherwise holos searches known
// distro paths by pair so doctor and VM launch agree on what "usable" means.
func ResolveOVMFFirmware() (OVMFFirmware, error) {
	codeEnv := os.Getenv(ovmfCodeEnv)
	varsEnv := os.Getenv(ovmfVarsEnv)
	if firmware, ok, err := resolveOVMFEnvOverride(codeEnv, varsEnv); ok || err != nil {
		return firmware, err
	}

	for i, codePath := range ovmfCodePaths {
		varsPath := ovmfVarsPaths[i]
		if readableOVMFPair(codePath, varsPath) {
			return OVMFFirmware{CodePath: codePath, VarsTemplatePath: varsPath}, nil
		}
	}

	return OVMFFirmware{}, fmt.Errorf("OVMF firmware CODE/VARS pair not found; install ovmf/edk2-ovmf or set %s and %s", ovmfCodeEnv, ovmfVarsEnv)
}

func resolveOVMFEnvOverride(codeEnv, varsEnv string) (OVMFFirmware, bool, error) {
	if codeEnv == "" && varsEnv == "" {
		return OVMFFirmware{}, false, nil
	}
	if ovmfEnvOverrideIncomplete(codeEnv, varsEnv) {
		return OVMFFirmware{}, true, fmt.Errorf("set both %s and %s, or neither", ovmfCodeEnv, ovmfVarsEnv)
	}
	if err := checkReadableFile(codeEnv); err != nil {
		return OVMFFirmware{}, true, fmt.Errorf("%s=%q is not usable: %w", ovmfCodeEnv, codeEnv, err)
	}
	if err := checkReadableFile(varsEnv); err != nil {
		return OVMFFirmware{}, true, fmt.Errorf("%s=%q is not usable: %w", ovmfVarsEnv, varsEnv, err)
	}
	return OVMFFirmware{CodePath: codeEnv, VarsTemplatePath: varsEnv}, true, nil
}

func ovmfEnvOverrideIncomplete(codeEnv, varsEnv string) bool {
	return (codeEnv == "") != (varsEnv == "")
}

func readableOVMFPair(codePath, varsPath string) bool {
	return checkReadableFile(codePath) == nil && checkReadableFile(varsPath) == nil
}

func checkReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, ovmfVarsPerm)
}
