package runtime

import (
	"fmt"
	"os"
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
