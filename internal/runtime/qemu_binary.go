package runtime

import (
	"errors"
	"os"
	"os/exec"
)

const (
	qemuSystemEnv     = "HOLOS_QEMU_SYSTEM"
	qemuImgEnv        = "HOLOS_QEMU_IMG"
	ipEnv             = "HOLOS_IP"
	qemuSystemDefault = "qemu-system-x86_64"
	qemuImgDefault    = "qemu-img"
	ipDefault         = "ip"
	qemuSystemHint    = "install QEMU/KVM"
	qemuImgHint       = "install QEMU tools"
	ipHint            = "install iproute2"
)

func (m *Manager) qemuSystemCommand(args ...string) (*exec.Cmd, error) {
	binary, err := m.qemuSystemBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(binary, args...), nil
}

func (m *Manager) qemuSystemBinary() (string, error) {
	return binaryFromEnvOrPath(qemuSystemEnv, qemuSystemDefault, qemuSystemHint)
}

func (m *Manager) qemuImgBinary() (string, error) {
	return binaryFromEnvOrPath(qemuImgEnv, qemuImgDefault, qemuImgHint)
}

func (m *Manager) ipBinary() (string, error) {
	return binaryFromEnvOrPath(ipEnv, ipDefault, ipHint)
}

func binaryFromEnvOrPath(envName, defaultBinary, installHint string) (string, error) {
	if value := os.Getenv(envName); value != "" {
		return value, nil
	}
	binary, err := exec.LookPath(defaultBinary)
	if err != nil {
		return "", missingBinaryError(envName, defaultBinary, installHint)
	}
	return binary, nil
}

func missingBinaryError(envName, defaultBinary, installHint string) error {
	return errors.New(defaultBinary + " not found; " + installHint + " or set " + envName)
}
