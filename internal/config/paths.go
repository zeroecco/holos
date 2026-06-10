package config

import (
	"fmt"
	"path/filepath"
)

func (m *Manifest) resolvePaths(baseDir string) error {
	if m.Image == "" {
		return nil
	}

	image, err := resolvePath(baseDir, m.Image)
	if err != nil {
		return fmt.Errorf("resolve image path: %w", err)
	}
	m.Image = image

	for i := range m.Mounts {
		// Only bind mounts resolve host paths; named volumes are
		// materialised by the runtime under state_dir/volumes/.
		if m.Mounts[i].Kind == MountKindVolume {
			continue
		}
		if m.Mounts[i].Source == "" {
			continue
		}
		source, err := resolvePath(baseDir, m.Mounts[i].Source)
		if err != nil {
			return fmt.Errorf("resolve mount %q: %w", m.Mounts[i].Source, err)
		}
		m.Mounts[i].Source = source
	}
	return nil
}

func resolvePath(baseDir, value string) (string, error) {
	if filepath.IsAbs(value) {
		return value, nil
	}
	absolute, err := filepath.Abs(filepath.Join(baseDir, value))
	if err != nil {
		return "", err
	}
	return absolute, nil
}
