package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

// SnapshotInstanceRoot creates an internal qcow2 snapshot on a stopped
// instance's root overlay.
func (m *Manager) SnapshotInstanceRoot(projectName, instanceName, snapshotName string) error {
	if err := compose.ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	return m.withProjectLock(projectName, func() error {
		return m.snapshotInstanceRootLocked(projectName, instanceName, snapshotName)
	})
}

func (m *Manager) snapshotInstanceRootLocked(projectName, instanceName, snapshotName string) error {
	record, err := m.projectStatusLocked(projectName)
	if err != nil {
		return err
	}
	inst, serviceName, ok := findInstanceRecord(record, instanceName)
	if !ok {
		return instanceNotFoundError(projectName, instanceName)
	}
	if inst.Status == InstanceStatusRunning {
		return fmt.Errorf("instance %q in project %q is running; stop it before snapshotting root overlay", instanceName, projectName)
	}
	if inst.OverlayPath == "" {
		return fmt.Errorf("instance %q in project %q has no root overlay path", instanceName, projectName)
	}
	if _, err := os.Stat(inst.OverlayPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("instance %q root overlay %s not found", instanceName, inst.OverlayPath)
		}
		return fmt.Errorf("stat instance %q root overlay %s: %w", instanceName, inst.OverlayPath, err)
	}

	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}
	if output, err := exec.Command(qemuImg, diskSnapshotCreateArgs(snapshotName, inst.OverlayPath)...).CombinedOutput(); err != nil {
		return fmt.Errorf("snapshot instance %q (%s) in project %q: %w: %s",
			instanceName, serviceName, projectName, err, strings.TrimSpace(string(output)))
	}
	return nil
}
