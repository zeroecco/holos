package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

// resolveVolumes gathers every named volume actually referenced by a
// service and returns them with their resolved sizes. Unreferenced
// top-level volumes are intentionally omitted so `holos down` never
// leaves behind qcow2 files for volumes nothing asked for. A reference
// to a volume that's not declared is an error (prevents typos from
// silently degrading to bind mounts).
func (f *File) resolveVolumes(services map[string]config.Manifest) (map[string]VolumeSpec, error) {
	used := make(map[string]bool)
	for name, manifest := range services {
		for _, m := range manifest.Mounts {
			if m.Kind != config.MountKindVolume {
				continue
			}
			if _, ok := f.Volumes[m.VolumeName]; !ok {
				return nil, fmt.Errorf(
					"service %q references volume %q not declared in top-level volumes:",
					name, m.VolumeName)
			}
			used[m.VolumeName] = true
		}
	}

	if len(used) == 0 {
		return nil, nil
	}

	out := make(map[string]VolumeSpec, len(used))
	for name := range used {
		size, err := parseVolumeSize(f.Volumes[name].Size)
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", name, err)
		}
		if !namePattern.MatchString(name) {
			return nil, fmt.Errorf("volume name %q must match %s", name, namePattern.String())
		}
		out[name] = VolumeSpec{Name: name, SizeBytes: size, SourcePath: volumeSourcePath(f.Volumes[name])}
	}
	return out, nil
}

func volumeSourcePath(volume Volume) string {
	if volume.DriverOpts == nil {
		return ""
	}
	return volume.DriverOpts["source"]
}
