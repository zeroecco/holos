package runtime

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/compose"
)

// VolumeInfo is the read-only inventory shape returned by ListVolumes.
type VolumeInfo struct {
	Project     string                 `json:"project"`
	Name        string                 `json:"name"`
	SizeBytes   int64                  `json:"size_bytes"`
	Path        string                 `json:"path"`
	Attachments []VolumeAttachmentInfo `json:"attachments,omitempty"`
}

// VolumeAttachmentInfo identifies an instance workdir that currently references
// a named volume backing file.
type VolumeAttachmentInfo struct {
	Service  string `json:"service"`
	Instance string `json:"instance"`
	Status   string `json:"status"`
}

type volumeKey struct {
	project string
	name    string
}

// ListVolumes returns all known named-volume backing files, enriched with
// declared sizes from project records and attachment state from instance
// workdirs.
func (m *Manager) ListVolumes() ([]VolumeInfo, error) {
	if err := m.ensureLayout(); err != nil {
		return nil, err
	}

	projects, err := m.ListProjects()
	if err != nil {
		return nil, err
	}

	volumes, err := m.volumeFiles()
	if err != nil {
		return nil, err
	}

	declared := declaredVolumeSizes(projects)
	attachments, err := volumeAttachments(projects)
	if err != nil {
		return nil, err
	}

	for key, size := range declared {
		info := volumes[key]
		info.Project = key.project
		info.Name = key.name
		info.Path = volumeBackingPath(m.stateDir, key.project, key.name)
		info.SizeBytes = size
		volumes[key] = info
	}
	for key, attached := range attachments {
		info := volumes[key]
		info.Project = key.project
		info.Name = key.name
		info.Path = volumeBackingPath(m.stateDir, key.project, key.name)
		info.Attachments = attached
		volumes[key] = info
	}

	out := make([]VolumeInfo, 0, len(volumes))
	for _, info := range volumes {
		out = append(out, info)
	}
	sortVolumeInfos(out)
	return out, nil
}

func volumeRecordsForProject(project *compose.Project) []VolumeRecord {
	records := make([]VolumeRecord, 0, len(project.Volumes))
	for _, spec := range project.Volumes {
		records = append(records, VolumeRecord{Name: spec.Name, SizeBytes: spec.SizeBytes})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})
	return records
}

func (m *Manager) volumeFiles() (map[volumeKey]VolumeInfo, error) {
	volumes := map[volumeKey]VolumeInfo{}
	root := filepath.Join(m.stateDir, volumesStateSubdir)
	projectDirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return volumes, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list volumes root: %w", err)
	}
	for _, projectDir := range projectDirs {
		if !projectDir.IsDir() {
			continue
		}
		project := projectDir.Name()
		files, err := os.ReadDir(filepath.Join(root, project))
		if err != nil {
			return nil, fmt.Errorf("list project volumes %q: %w", project, err)
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != volumeDiskExtension {
				continue
			}
			info, err := file.Info()
			if err != nil {
				return nil, fmt.Errorf("stat volume %q/%q: %w", project, file.Name(), err)
			}
			name := strings.TrimSuffix(file.Name(), volumeDiskExtension)
			key := volumeKey{project: project, name: name}
			volumes[key] = VolumeInfo{
				Project:   project,
				Name:      name,
				SizeBytes: info.Size(),
				Path:      volumeBackingPath(m.stateDir, project, name),
			}
		}
	}
	return volumes, nil
}

func declaredVolumeSizes(projects []*ProjectRecord) map[volumeKey]int64 {
	out := map[volumeKey]int64{}
	for _, project := range projects {
		for _, volume := range project.Volumes {
			out[volumeKey{project: project.Name, name: volume.Name}] = volume.SizeBytes
		}
	}
	return out
}

func volumeAttachments(projects []*ProjectRecord) (map[volumeKey][]VolumeAttachmentInfo, error) {
	out := map[volumeKey][]VolumeAttachmentInfo{}
	for _, project := range projects {
		for _, service := range project.Services {
			for _, instance := range service.Instances {
				attached, err := instanceVolumeAttachments(project.Name, service.Name, instance)
				if err != nil {
					return nil, err
				}
				for key, attachment := range attached {
					out[key] = append(out[key], attachment)
				}
			}
		}
	}
	for key := range out {
		sort.Slice(out[key], func(i, j int) bool {
			if out[key][i].Service != out[key][j].Service {
				return out[key][i].Service < out[key][j].Service
			}
			return out[key][i].Instance < out[key][j].Instance
		})
	}
	return out, nil
}

func instanceVolumeAttachments(project, service string, instance InstanceRecord) (map[volumeKey]VolumeAttachmentInfo, error) {
	out := map[volumeKey]VolumeAttachmentInfo{}
	if instance.WorkDir == "" {
		return out, nil
	}
	entries, err := os.ReadDir(instance.WorkDir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list instance volumes %q: %w", instance.Name, err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		name, ok := volumeNameFromLink(entry.Name())
		if !ok {
			continue
		}
		out[volumeKey{project: project, name: name}] = VolumeAttachmentInfo{
			Service:  service,
			Instance: instance.Name,
			Status:   instance.Status,
		}
	}
	return out, nil
}

func volumeNameFromLink(name string) (string, bool) {
	if !strings.HasPrefix(name, volumeLinkPrefix) || !strings.HasSuffix(name, volumeDiskExtension) {
		return "", false
	}
	volume := strings.TrimSuffix(strings.TrimPrefix(name, volumeLinkPrefix), volumeDiskExtension)
	return volume, volume != ""
}

func sortVolumeInfos(volumes []VolumeInfo) {
	sort.Slice(volumes, func(i, j int) bool {
		if volumes[i].Project != volumes[j].Project {
			return volumes[i].Project < volumes[j].Project
		}
		return volumes[i].Name < volumes[j].Name
	})
}

// RemoveVolume deletes a detached named-volume backing file.
func (m *Manager) RemoveVolume(projectName, volumeName string) error {
	if err := compose.ValidateName(volumeName); err != nil {
		return fmt.Errorf("invalid volume name: %w", err)
	}
	return m.withProjectLock(projectName, func() error {
		return m.removeVolumeLocked(projectName, volumeName)
	})
}

func (m *Manager) removeVolumeLocked(projectName, volumeName string) error {
	volume, ok, err := m.findVolume(projectName, volumeName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
	}
	if len(volume.Attachments) > 0 {
		return fmt.Errorf("volume %q in project %q is attached to %s", volumeName, projectName, volumeAttachmentSummary(volume.Attachments))
	}
	if err := os.Remove(volumeBackingPath(m.stateDir, projectName, volumeName)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
		}
		return fmt.Errorf("remove volume %q in project %q: %w", volumeName, projectName, err)
	}
	return nil
}

func (m *Manager) findVolume(projectName, volumeName string) (VolumeInfo, bool, error) {
	volumes, err := m.ListVolumes()
	if err != nil {
		return VolumeInfo{}, false, err
	}
	for _, volume := range volumes {
		if volume.Project == projectName && volume.Name == volumeName {
			return volume, true, nil
		}
	}
	return VolumeInfo{}, false, nil
}

func volumeAttachmentSummary(attachments []VolumeAttachmentInfo) string {
	parts := make([]string, len(attachments))
	for i, attachment := range attachments {
		parts[i] = attachment.Instance + ":" + attachment.Status
	}
	return strings.Join(parts, ",")
}

// ExportVolume copies a detached named-volume backing file to destination.
// It refuses to overwrite existing files.
func (m *Manager) ExportVolume(projectName, volumeName, destination string) error {
	if err := compose.ValidateName(volumeName); err != nil {
		return fmt.Errorf("invalid volume name: %w", err)
	}
	if destination == "" {
		return fmt.Errorf("export destination is required")
	}
	return m.withProjectLock(projectName, func() error {
		return m.exportVolumeLocked(projectName, volumeName, destination)
	})
}

func (m *Manager) exportVolumeLocked(projectName, volumeName, destination string) error {
	volume, ok, err := m.findVolume(projectName, volumeName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
	}
	if len(volume.Attachments) > 0 {
		return fmt.Errorf("volume %q in project %q is attached to %s", volumeName, projectName, volumeAttachmentSummary(volume.Attachments))
	}

	src, err := os.Open(volumeBackingPath(m.stateDir, projectName, volumeName))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
		}
		return fmt.Errorf("open volume %q in project %q: %w", volumeName, projectName, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("export destination %q already exists", destination)
		}
		return fmt.Errorf("create export destination %q: %w", destination, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("copy volume %q to %q: %w", volumeName, destination, err)
	}
	return nil
}

// SnapshotVolume creates an internal qcow2 snapshot on a detached named-volume
// backing file.
func (m *Manager) SnapshotVolume(projectName, volumeName, snapshotName string) error {
	if err := compose.ValidateName(volumeName); err != nil {
		return fmt.Errorf("invalid volume name: %w", err)
	}
	if err := compose.ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	return m.withProjectLock(projectName, func() error {
		return m.snapshotVolumeLocked(projectName, volumeName, snapshotName)
	})
}

// ListVolumeSnapshots lists internal snapshots on a detached named volume.
func (m *Manager) ListVolumeSnapshots(projectName, volumeName string) ([]SnapshotInfo, error) {
	return m.volumeSnapshots(projectName, volumeName, false, "")
}

// RemoveVolumeSnapshot deletes an internal snapshot from a detached named volume.
func (m *Manager) RemoveVolumeSnapshot(projectName, volumeName, snapshotName string) error {
	if err := compose.ValidateName(volumeName); err != nil {
		return fmt.Errorf("invalid volume name: %w", err)
	}
	if err := compose.ValidateName(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot name: %w", err)
	}
	_, err := m.volumeSnapshots(projectName, volumeName, true, snapshotName)
	return err
}

func (m *Manager) volumeSnapshots(projectName, volumeName string, remove bool, snapshotName string) ([]SnapshotInfo, error) {
	var snapshots []SnapshotInfo
	err := m.withProjectLock(projectName, func() error {
		volume, ok, err := m.findVolume(projectName, volumeName)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
		}
		if len(volume.Attachments) > 0 {
			return fmt.Errorf("volume %q in project %q is attached to %s", volumeName, projectName, volumeAttachmentSummary(volume.Attachments))
		}
		qemuImg, err := m.qemuImgBinary()
		if err != nil {
			return err
		}
		path := volumeBackingPath(m.stateDir, projectName, volumeName)
		if remove {
			output, err := exec.Command(qemuImg, diskSnapshotDeleteArgs(snapshotName, path)...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("remove snapshot %q: %w: %s", snapshotName, err, strings.TrimSpace(string(output)))
			}
			return nil
		}
		output, err := exec.Command(qemuImg, diskSnapshotListArgs(path)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("list snapshots: %w: %s", err, strings.TrimSpace(string(output)))
		}
		snapshots = parseSnapshotList(string(output))
		return nil
	})
	return snapshots, err
}

func (m *Manager) snapshotVolumeLocked(projectName, volumeName, snapshotName string) error {
	volume, ok, err := m.findVolume(projectName, volumeName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
	}
	if len(volume.Attachments) > 0 {
		return fmt.Errorf("volume %q in project %q is attached to %s", volumeName, projectName, volumeAttachmentSummary(volume.Attachments))
	}

	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}
	path := volumeBackingPath(m.stateDir, projectName, volumeName)
	if output, err := exec.Command(qemuImg, diskSnapshotCreateArgs(snapshotName, path)...).CombinedOutput(); err != nil {
		return fmt.Errorf("snapshot volume %q in project %q: %w: %s",
			volumeName, projectName, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func diskSnapshotCreateArgs(snapshotName, path string) []string {
	return []string{qemuImgSnapshotSubcommand, qemuImgSnapshotCreateFlag, snapshotName, path}
}

// ResizeVolume changes the virtual size of a detached named-volume backing
// file and updates the saved project volume record when one exists.
func (m *Manager) ResizeVolume(projectName, volumeName string, sizeBytes int64, allowShrink bool) error {
	if err := compose.ValidateName(volumeName); err != nil {
		return fmt.Errorf("invalid volume name: %w", err)
	}
	if sizeBytes <= 0 {
		return fmt.Errorf("volume size must be positive")
	}
	return m.withProjectLock(projectName, func() error {
		return m.resizeVolumeLocked(projectName, volumeName, sizeBytes, allowShrink)
	})
}

func (m *Manager) resizeVolumeLocked(projectName, volumeName string, sizeBytes int64, allowShrink bool) error {
	volume, ok, err := m.findVolume(projectName, volumeName)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("volume %q not found in project %q", volumeName, projectName)
	}
	if len(volume.Attachments) > 0 {
		return fmt.Errorf("volume %q in project %q is attached to %s", volumeName, projectName, volumeAttachmentSummary(volume.Attachments))
	}

	qemuImg, err := m.qemuImgBinary()
	if err != nil {
		return err
	}
	path := volumeBackingPath(m.stateDir, projectName, volumeName)
	if output, err := exec.Command(qemuImg, volumeResizeArgs(path, sizeBytes, allowShrink)...).CombinedOutput(); err != nil {
		return fmt.Errorf("resize volume %q in project %q: %w: %s",
			volumeName, projectName, err, strings.TrimSpace(string(output)))
	}
	return m.updateVolumeRecordSize(projectName, volumeName, sizeBytes)
}

func volumeResizeArgs(path string, sizeBytes int64, allowShrink bool) []string {
	args := []string{qemuImgResizeSubcommand}
	if allowShrink {
		args = append(args, qemuImgResizeShrinkFlag)
	}
	return append(args, path, byteSizeArg(sizeBytes))
}

func (m *Manager) updateVolumeRecordSize(projectName, volumeName string, sizeBytes int64) error {
	record, err := m.loadProject(projectName)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := false
	for i := range record.Volumes {
		if record.Volumes[i].Name == volumeName {
			record.Volumes[i].SizeBytes = sizeBytes
			updated = true
			break
		}
	}
	if !updated {
		record.Volumes = append(record.Volumes, VolumeRecord{Name: volumeName, SizeBytes: sizeBytes})
	}
	return m.saveUpdatedProject(record)
}
