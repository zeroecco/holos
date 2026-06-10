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

const (
	volumesStateSubdir  = "volumes"
	volumeDiskExtension = ".qcow2"
	volumeLinkPrefix    = "vol-"
)

// volumesRoot returns the per-project directory where qcow2 backing files
// for named volumes live. Separated from the per-instance workdir so that
// `holos down` (which rm -rf's workdirs) never touches volume data.
func volumesRoot(stateDir, project string) string {
	return filepath.Join(stateDir, volumesStateSubdir, project)
}

// volumeBackingPath is the on-disk qcow2 path for a named volume.
func volumeBackingPath(stateDir, project, name string) string {
	return filepath.Join(volumesRoot(stateDir, project), name+volumeDiskExtension)
}

func volumeLinkPath(workDir, name string) string {
	return filepath.Join(workDir, volumeLinkName(name))
}

func volumeLinkName(name string) string {
	return volumeLinkPrefix + name + volumeDiskExtension
}

// ensureProjectVolumes creates any missing qcow2 backing files for the
// project's declared volumes. Idempotent: an existing file of any size
// is kept as-is (resizing volumes is a separate, destructive operation).
func (m *Manager) ensureProjectVolumes(project *compose.Project) error {
	if len(project.Volumes) == 0 {
		return nil
	}

	root := volumesRoot(m.stateDir, project.Name)
	// Volume qcow2 backing files contain whatever the guest wrote to
	// the mount; treat the whole tree as private to the holos user.
	if err := os.MkdirAll(root, stateDirPerm); err != nil {
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

		if output, err := exec.Command(qemuImg, volumeCreateArgs(path, spec.SizeBytes)...).CombinedOutput(); err != nil {
			_ = os.Remove(path)
			return fmt.Errorf("create volume %s: %w: %s",
				spec.Name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func volumeCreateArgs(path string, sizeBytes int64) []string {
	return []string{qemuImgCreateSubcommand, qemuImgFormatFlag, config.ImageFormatQCOW2, path, byteSizeArg(sizeBytes)}
}

// materializeInstanceVolumes turns a service's named-volume mounts into
// qemu attachments by symlinking each backing qcow2 into the instance
// workdir. Teardown (os.RemoveAll(workdir)) removes the symlinks but
// leaves the target files untouched so volume data survives `holos down`.
func materializeInstanceVolumes(stateDir, project, workDir string, mounts []config.Mount) ([]qemu.VolumeAttachment, error) {
	var attachments []qemu.VolumeAttachment
	for _, mount := range mounts {
		attachment, ok, err := materializeInstanceVolume(stateDir, project, workDir, mount)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func materializeInstanceVolume(stateDir, project, workDir string, mount config.Mount) (qemu.VolumeAttachment, bool, error) {
	if mount.Kind != config.MountKindVolume {
		return qemu.VolumeAttachment{}, false, nil
	}
	backing := volumeBackingPath(stateDir, project, mount.VolumeName)
	if _, err := os.Stat(backing); err != nil {
		return qemu.VolumeAttachment{}, false, fmt.Errorf("volume %q backing %s missing: %w",
			mount.VolumeName, backing, err)
	}

	link := volumeLinkPath(workDir, mount.VolumeName)
	// Remove any stale link from a previous run of this instance
	// (for example after a crash that left the workdir in place).
	_ = os.Remove(link)
	if err := os.Symlink(backing, link); err != nil {
		return qemu.VolumeAttachment{}, false, fmt.Errorf("symlink volume %q: %w", mount.VolumeName, err)
	}

	return volumeAttachmentForMount(mount, link), true, nil
}

func volumeAttachmentForMount(mount config.Mount, diskPath string) qemu.VolumeAttachment {
	return qemu.VolumeAttachment{
		Name:     mount.VolumeName,
		DiskPath: diskPath,
		ReadOnly: mount.ReadOnly,
	}
}
