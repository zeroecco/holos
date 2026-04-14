package runtime

import (
	"fmt"
	"os"
	"path/filepath"
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
	codePath, err = findOVMF("HOLOS_OVMF_CODE", ovmfCodePaths)
	if err != nil {
		return "", "", err
	}

	templatePath, err := findOVMF("HOLOS_OVMF_VARS", ovmfVarsPaths)
	if err != nil {
		return "", "", err
	}

	varsPath = filepath.Join(workDir, "OVMF_VARS.fd")
	if err := copyFile(templatePath, varsPath); err != nil {
		return "", "", fmt.Errorf("copy OVMF_VARS: %w", err)
	}

	return codePath, varsPath, nil
}

func findOVMF(envVar string, searchPaths []string) (string, error) {
	if value := os.Getenv(envVar); value != "" {
		if _, err := os.Stat(value); err == nil {
			return value, nil
		}
		return "", fmt.Errorf("%s=%q not found", envVar, value)
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("OVMF firmware not found; install ovmf/edk2-ovmf or set %s", envVar)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
