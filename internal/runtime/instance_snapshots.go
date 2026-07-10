package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

// SnapshotInfo identifies an internal qcow2 snapshot.
type SnapshotInfo struct {
	Name string `json:"name"`
}

func validateSnapshotName(snapshotName string) error {
	if err := compose.ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	return nil
}

// SnapshotInstanceRoot creates an internal qcow2 snapshot on a stopped
// instance's root overlay.
func (m *Manager) SnapshotInstanceRoot(projectName, instanceName, snapshotName string) error {
	if err := validateSnapshotName(snapshotName); err != nil {
		return err
	}
	return m.withProjectLock(projectName, func() error {
		return m.snapshotInstanceRootLocked(projectName, instanceName, snapshotName)
	})
}

// ListInstanceSnapshots lists internal snapshots on a stopped instance root
// overlay. Snapshot metadata lives inside qcow2, so no extra holos state file
// is required.
func (m *Manager) ListInstanceSnapshots(projectName, instanceName string) ([]SnapshotInfo, error) {
	return m.instanceSnapshots(projectName, instanceName, false, "")
}

// RemoveInstanceSnapshot deletes an internal snapshot from a stopped instance.
func (m *Manager) RemoveInstanceSnapshot(projectName, instanceName, snapshotName string) error {
	if err := validateSnapshotName(snapshotName); err != nil {
		return err
	}
	_, err := m.instanceSnapshots(projectName, instanceName, true, snapshotName)
	return err
}

// RestoreInstanceSnapshot applies an internal snapshot to a stopped instance
// root overlay. The active image is changed in place.
func (m *Manager) RestoreInstanceSnapshot(projectName, instanceName, snapshotName string) error {
	if err := validateSnapshotName(snapshotName); err != nil {
		return err
	}
	return m.withProjectLock(projectName, func() error {
		path, err := m.instanceSnapshotPathLocked(projectName, instanceName)
		if err != nil {
			return err
		}
		return m.applySnapshot(path, snapshotName)
	})
}

// ExportInstanceSnapshot writes a standalone qcow2 image containing the state
// represented by an internal snapshot. It refuses to overwrite a destination.
func (m *Manager) ExportInstanceSnapshot(projectName, instanceName, snapshotName, destination string) error {
	if err := validateSnapshotName(snapshotName); err != nil {
		return err
	}
	return m.withProjectLock(projectName, func() error {
		path, err := m.instanceSnapshotPathLocked(projectName, instanceName)
		if err != nil {
			return err
		}
		return m.exportSnapshot(path, snapshotName, destination)
	})
}

func (m *Manager) instanceSnapshotPathLocked(projectName, instanceName string) (string, error) {
	record, err := m.projectStatusLocked(projectName)
	if err != nil {
		return "", err
	}
	inst, _, ok := findInstanceRecord(record, instanceName)
	if !ok {
		return "", instanceNotFoundError(projectName, instanceName)
	}
	if inst.Status == InstanceStatusRunning {
		return "", fmt.Errorf("instance %q in project %q is running; stop it first", instanceName, projectName)
	}
	if inst.OverlayPath == "" {
		return "", fmt.Errorf("instance %q has no root overlay path", instanceName)
	}
	if _, err := os.Stat(inst.OverlayPath); err != nil {
		return "", fmt.Errorf("stat instance root overlay: %w", err)
	}
	return inst.OverlayPath, nil
}

func (m *Manager) applySnapshot(path, snapshotName string) error {
	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}
	output, err := exec.Command(qemuImg, diskSnapshotApplyArgs(snapshotName, path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apply snapshot %q: %w: %s", snapshotName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) exportSnapshot(source, snapshotName, destination string) error {
	if destination == "" {
		return fmt.Errorf("snapshot export destination is required")
	}
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("export destination %q already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat export destination %q: %w", destination, err)
	}
	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}
	output, err := exec.Command(qemuImg, diskSnapshotExportArgs(snapshotName, source, destination)...).CombinedOutput()
	if err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("export snapshot %q: %w: %s", snapshotName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) instanceSnapshots(projectName, instanceName string, remove bool, snapshotName string) ([]SnapshotInfo, error) {
	var out []SnapshotInfo
	err := m.withProjectLock(projectName, func() error {
		record, err := m.projectStatusLocked(projectName)
		if err != nil {
			return err
		}
		inst, _, ok := findInstanceRecord(record, instanceName)
		if !ok {
			return instanceNotFoundError(projectName, instanceName)
		}
		if inst.Status == InstanceStatusRunning {
			return fmt.Errorf("instance %q in project %q is running; stop it first", instanceName, projectName)
		}
		if inst.OverlayPath == "" {
			return fmt.Errorf("instance %q has no root overlay path", instanceName)
		}
		if _, err := os.Stat(inst.OverlayPath); err != nil {
			return fmt.Errorf("stat instance root overlay: %w", err)
		}
		qemuImg, err := m.qemuImgBinary()
		if err != nil {
			return err
		}
		if remove {
			if output, err := exec.Command(qemuImg, diskSnapshotDeleteArgs(snapshotName, inst.OverlayPath)...).CombinedOutput(); err != nil {
				return fmt.Errorf("remove snapshot %q: %w: %s", snapshotName, err, strings.TrimSpace(string(output)))
			}
			return nil
		}
		output, err := exec.Command(qemuImg, diskSnapshotListArgs(inst.OverlayPath)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("list snapshots: %w: %s", err, strings.TrimSpace(string(output)))
		}
		out = parseSnapshotList(string(output))
		return nil
	})
	return out, err
}

func parseSnapshotList(output string) []SnapshotInfo {
	seen := map[string]bool{}
	var out []SnapshotInfo
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "ID" || !isDecimal(fields[0]) {
			continue
		}
		if !seen[fields[1]] {
			out = append(out, SnapshotInfo{Name: fields[1]})
			seen[fields[1]] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
