package config

import "path/filepath"

func (m *Manifest) applyDefaults() {
	if m.APIVersion == "" {
		m.APIVersion = DefaultAPIVersion
	}
	if m.Kind == "" {
		m.Kind = DefaultKind
	}
	if m.Replicas == 0 {
		m.Replicas = DefaultReplicas
	}
	if m.ImageFormat == "" {
		m.ImageFormat = inferImageFormat(m.Image)
	}
	if m.VM.VCPU == 0 {
		m.VM.VCPU = DefaultVCPU
	}
	if m.VM.MemoryMB == 0 {
		m.VM.MemoryMB = DefaultMemoryMB
	}
	if m.VM.Machine == "" {
		m.VM.Machine = DefaultMachine
	}
	if m.VM.CPUModel == "" {
		m.VM.CPUModel = DefaultCPUModel
	}
	if m.Network.Mode == "" {
		m.Network.Mode = DefaultNetworkMode
	}
	if m.CloudInit.User == "" {
		m.CloudInit.User = DefaultUser
	}
	if m.StopGracePeriodSec == 0 {
		m.StopGracePeriodSec = DefaultStopGracePeriodSec
	}
	if m.Healthcheck != nil {
		applyHealthcheckDefaults(m.Healthcheck)
	}
	for i := range m.Ports {
		applyPortDefaults(&m.Ports[i])
	}
	for i := range m.CloudInit.WriteFiles {
		applyWriteFileDefaults(&m.CloudInit.WriteFiles[i])
	}
	for i := range m.Mounts {
		applyMountDefaults(&m.Mounts[i])
	}
}

func applyHealthcheckDefaults(healthcheck *HealthcheckConfig) {
	healthcheck.IntervalSec = intOrDefault(healthcheck.IntervalSec, DefaultHealthIntervalSec)
	healthcheck.StartIntervalSec = intOrDefault(healthcheck.StartIntervalSec, healthcheck.IntervalSec)
	healthcheck.Retries = intOrDefault(healthcheck.Retries, DefaultHealthRetries)
	healthcheck.TimeoutSec = intOrDefault(healthcheck.TimeoutSec, DefaultHealthTimeoutSec)
}

func intOrDefault(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

func applyPortDefaults(port *PortForward) {
	if port.Protocol == "" {
		port.Protocol = DefaultProtocol
	}
}

func applyMountDefaults(mount *Mount) {
	if mount.Kind == "" {
		mount.Kind = MountKindBind
	}
}

func applyWriteFileDefaults(file *WriteFile) {
	if file.Permissions == "" {
		file.Permissions = DefaultFilePermissions
	}
	if file.Owner == "" {
		file.Owner = DefaultFileOwner
	}
}

func inferImageFormat(path string) string {
	switch filepath.Ext(path) {
	case ".raw", ".img":
		return ImageFormatRaw
	default:
		return ImageFormatQCOW2
	}
}
