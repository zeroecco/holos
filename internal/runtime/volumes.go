package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

// volumesRoot returns the per-project directory where qcow2 backing files
// for named volumes live. Separated from the per-instance workdir so that
// `holos down` (which rm -rf's workdirs) never touches volume data.
func volumesRoot(stateDir, project string) string {
	return filepath.Join(stateDir, "volumes", project)
}

// volumeBackingPath is the on-disk qcow2 path for a named volume.
func volumeBackingPath(stateDir, project, name string) string {
	return filepath.Join(volumesRoot(stateDir, project), name+".qcow2")
}

// ensureProjectVolumes creates any missing qcow2 backing files for the
// project's declared volumes. Idempotent: an existing file of any size
// is kept as-is (resizing volumes is a separate, destructive operation).
func (m *Manager) ensureProjectVolumes(project *compose.Project) error {
	if len(project.Volumes) == 0 {
		return nil
	}

	root := volumesRoot(m.stateDir, project.Name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create volumes dir: %w", err)
	}

	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}

	for _, spec := range project.Volumes {
		path := volumeBackingPath(m.stateDir, project.Name, spec.Name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat volume %s: %w", path, err)
		}

		args := []string{"create", "-f", "qcow2", path, fmt.Sprintf("%d", spec.SizeBytes)}
		if output, err := exec.Command(qemuImg, args...).CombinedOutput(); err != nil {
			_ = os.Remove(path)
			return fmt.Errorf("create volume %s: %w: %s",
				spec.Name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// materializeInstanceVolumes turns a service's named-volume mounts into
// qemu attachments by symlinking each backing qcow2 into the instance
// workdir. Teardown (os.RemoveAll(workdir)) removes the symlinks but
// leaves the target files untouched so volume data survives `holos down`.
func materializeInstanceVolumes(stateDir, project, workDir string, mounts []config.Mount) ([]qemu.VolumeAttachment, error) {
	var attachments []qemu.VolumeAttachment
	for _, mount := range mounts {
		if mount.Kind != config.MountKindVolume {
			continue
		}
		backing := volumeBackingPath(stateDir, project, mount.VolumeName)
		if _, err := os.Stat(backing); err != nil {
			return nil, fmt.Errorf("volume %q backing %s missing: %w",
				mount.VolumeName, backing, err)
		}

		link := filepath.Join(workDir, "vol-"+mount.VolumeName+".qcow2")
		// Remove any stale link from a previous run of this instance
		// (for example after a crash that left the workdir in place).
		_ = os.Remove(link)
		if err := os.Symlink(backing, link); err != nil {
			return nil, fmt.Errorf("symlink volume %q: %w", mount.VolumeName, err)
		}

		attachments = append(attachments, qemu.VolumeAttachment{
			Name:     mount.VolumeName,
			DiskPath: link,
		})
	}
	return attachments, nil
}
