package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

// OVMFFirmware is the resolved CODE/VARS firmware pair used for UEFI boots.
type OVMFFirmware struct {
	CodePath         string
	VarsTemplatePath string
}

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

	varsPath = filepath.Join(workDir, "OVMF_VARS.fd")
	if err := copyFile(firmware.VarsTemplatePath, varsPath); err != nil {
		return "", "", fmt.Errorf("copy OVMF_VARS: %w", err)
	}

	return firmware.CodePath, varsPath, nil
}

// ResolveOVMFFirmware locates a usable OVMF CODE/VARS template pair. If either
// environment override is set, both must be set; otherwise holos searches known
// distro paths by pair so doctor and VM launch agree on what "usable" means.
func ResolveOVMFFirmware() (OVMFFirmware, error) {
	codeEnv := os.Getenv("HOLOS_OVMF_CODE")
	varsEnv := os.Getenv("HOLOS_OVMF_VARS")
	if codeEnv != "" || varsEnv != "" {
		if codeEnv == "" || varsEnv == "" {
			return OVMFFirmware{}, fmt.Errorf("set both HOLOS_OVMF_CODE and HOLOS_OVMF_VARS, or neither")
		}
		if err := checkReadableFile(codeEnv); err != nil {
			return OVMFFirmware{}, fmt.Errorf("HOLOS_OVMF_CODE=%q is not usable: %w", codeEnv, err)
		}
		if err := checkReadableFile(varsEnv); err != nil {
			return OVMFFirmware{}, fmt.Errorf("HOLOS_OVMF_VARS=%q is not usable: %w", varsEnv, err)
		}
		return OVMFFirmware{CodePath: codeEnv, VarsTemplatePath: varsEnv}, nil
	}

	for i, codePath := range ovmfCodePaths {
		varsPath := ovmfVarsPaths[i]
		if checkReadableFile(codePath) == nil && checkReadableFile(varsPath) == nil {
			return OVMFFirmware{CodePath: codePath, VarsTemplatePath: varsPath}, nil
		}
	}

	return OVMFFirmware{}, fmt.Errorf("OVMF firmware CODE/VARS pair not found; install ovmf/edk2-ovmf or set HOLOS_OVMF_CODE and HOLOS_OVMF_VARS")
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
	return os.WriteFile(dst, data, 0o644)
}
