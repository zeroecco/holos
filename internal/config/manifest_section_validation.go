package config

import "fmt"

const (
	minHealthcheckIntervalSec    = 1
	minHealthcheckRetries        = 1
	minHealthcheckTimeoutSec     = 1
	minHealthcheckStartPeriodSec = 0
)

func validateHealthcheck(healthcheck *HealthcheckConfig) error {
	if healthcheck == nil {
		return nil
	}
	return validateHealthcheckConfig(*healthcheck)
}

func validateHealthcheckConfig(healthcheck HealthcheckConfig) error {
	if len(healthcheck.Test) == 0 {
		return fmt.Errorf("healthcheck.test is required")
	}
	if healthcheck.IntervalSec < minHealthcheckIntervalSec {
		return fmt.Errorf("healthcheck.interval_sec must be >= %d", minHealthcheckIntervalSec)
	}
	if healthcheck.Retries < minHealthcheckRetries {
		return fmt.Errorf("healthcheck.retries must be >= %d", minHealthcheckRetries)
	}
	if healthcheck.TimeoutSec < minHealthcheckTimeoutSec {
		return fmt.Errorf("healthcheck.timeout_sec must be >= %d", minHealthcheckTimeoutSec)
	}
	if healthcheck.StartPeriodSec < minHealthcheckStartPeriodSec {
		return fmt.Errorf("healthcheck.start_period_sec must be >= %d", minHealthcheckStartPeriodSec)
	}
	if healthcheck.StartIntervalSec != 0 && healthcheck.StartIntervalSec < minHealthcheckIntervalSec {
		return fmt.Errorf("healthcheck.start_interval_sec must be >= %d", minHealthcheckIntervalSec)
	}
	return nil
}

func validateMounts(mounts []Mount) error {
	for _, mount := range mounts {
		if err := validateMount(mount); err != nil {
			return err
		}
	}
	return nil
}

func validateMount(mount Mount) error {
	if mount.Target == "" {
		return fmt.Errorf("mounts require target")
	}
	switch mount.Kind {
	case "", MountKindBind:
		if mount.Source == "" {
			return fmt.Errorf("bind mount %q requires source", mount.Target)
		}
	case MountKindVolume:
		if err := validateVolumeMount(mount); err != nil {
			return err
		}
	default:
		return fmt.Errorf("mount %q: unknown kind %q", mount.Target, mount.Kind)
	}
	return nil
}

func validateVolumeMount(mount Mount) error {
	if mount.VolumeName == "" {
		return fmt.Errorf("volume mount %q requires volume_name", mount.Target)
	}
	if mount.SizeBytes < minVolumeSizeBytes {
		return fmt.Errorf("volume %q size_bytes %d is below minimum %d",
			mount.VolumeName, mount.SizeBytes, minVolumeSizeBytes)
	}
	return nil
}

func validateWriteFiles(files []WriteFile) error {
	for _, file := range files {
		if err := validateWriteFile(file); err != nil {
			return err
		}
	}
	return nil
}

func validateWriteFile(file WriteFile) error {
	if file.Path == "" {
		return fmt.Errorf("write_files entries require path")
	}
	return nil
}
