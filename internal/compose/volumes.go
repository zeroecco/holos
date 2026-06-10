package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

const (
	composeVolumeTypeBind    = "bind"
	composeVolumeTypeVolume  = "volume"
	composeVolumeTypeTmpfs   = "tmpfs"
	composeVolumeTypeImage   = "image"
	composeVolumeTypeNpipe   = "npipe"
	composeVolumeTypeCluster = "cluster"
)

func parseVolumes(specs []ComposeVolume, baseDir string, declared map[string]Volume) ([]config.Mount, error) {
	mounts := make([]config.Mount, 0, len(specs))
	for i, spec := range specs {
		mount, ok, err := parseComposeVolume(spec, baseDir, declared)
		if err != nil {
			return nil, fmt.Errorf("volume %d: %w", i, err)
		}
		if !ok {
			continue
		}
		mounts = append(mounts, mount)
	}
	return mounts, nil
}

func parseComposeVolume(spec ComposeVolume, baseDir string, declared map[string]Volume) (config.Mount, bool, error) {
	if spec.Short != "" {
		mount, err := parseVolume(spec.Short, baseDir, declared)
		return mount, true, err
	}
	typ := composeVolumeType(spec)
	switch typ {
	case composeVolumeTypeBind, composeVolumeTypeVolume:
		if spec.Source == "" || spec.Target == "" {
			return config.Mount{}, false, fmt.Errorf("%s volume requires source and target", typ)
		}
		mount, err := parseVolume(longVolumeSpec(spec), baseDir, declared)
		return mount, true, err
	case composeVolumeTypeTmpfs, composeVolumeTypeImage, composeVolumeTypeNpipe, composeVolumeTypeCluster:
		return config.Mount{}, false, nil
	default:
		return config.Mount{}, false, fmt.Errorf("unsupported volume type %q", spec.Type)
	}
}

func composeVolumeType(spec ComposeVolume) string {
	if spec.Type == "" {
		return composeVolumeTypeVolume
	}
	return spec.Type
}

func longVolumeMode(spec ComposeVolume) string {
	if spec.ReadOnly {
		return volumeModeReadOnly
	}
	return volumeModeReadWrite
}

func longVolumeSpec(spec ComposeVolume) string {
	return spec.Source + volumeSpecSeparator + spec.Target + volumeSpecSeparator + longVolumeMode(spec)
}
