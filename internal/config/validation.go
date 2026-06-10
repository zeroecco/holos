package config

import (
	"fmt"
	"regexp"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var userNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var pciAddressPattern = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)
var macAddressPattern = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)

const (
	minManifestReplicas   = 1
	minManifestVCPU       = 1
	minManifestMemoryMB   = 128
	minStopGracePeriodSec = 0
)

// Validate checks that all manifest fields are within acceptable ranges and
// formats. Returns the first validation error encountered, or nil.
func (m Manifest) Validate() error {
	if !serviceNamePattern.MatchString(m.Name) {
		return fmt.Errorf("name %q must match %s", m.Name, serviceNamePattern.String())
	}
	if m.Replicas < minManifestReplicas {
		return fmt.Errorf("replicas must be >= %d", minManifestReplicas)
	}
	if m.Image == "" {
		return fmt.Errorf("image is required")
	}
	if m.ImageFormat != ImageFormatQCOW2 && m.ImageFormat != ImageFormatRaw {
		return fmt.Errorf("image_format must be one of %s or %s", ImageFormatQCOW2, ImageFormatRaw)
	}
	if m.ImageOS != "" && m.ImageOS != ImageOSSystemd && m.ImageOS != ImageOSOpenRC {
		return fmt.Errorf("image_os must be one of %s or %s", ImageOSSystemd, ImageOSOpenRC)
	}
	if m.VM.VCPU < minManifestVCPU {
		return fmt.Errorf("vm.vcpu must be >= %d", minManifestVCPU)
	}
	if m.VM.MemoryMB < minManifestMemoryMB {
		return fmt.Errorf("vm.memory_mb must be >= %d", minManifestMemoryMB)
	}
	if m.VM.DiskSizeBytes != 0 && m.VM.DiskSizeBytes < minVolumeSizeBytes {
		return fmt.Errorf("vm.disk_size_bytes must be 0 or >= %d", minVolumeSizeBytes)
	}
	if m.Network.Mode != DefaultNetworkMode {
		return fmt.Errorf("network.mode %q is unsupported; only %s is implemented", m.Network.Mode, DefaultNetworkMode)
	}
	if err := validateInternalNetwork(m.InternalNetwork); err != nil {
		return err
	}
	if err := ValidateUserName(m.CloudInit.User); err != nil {
		return fmt.Errorf("cloud_init.user: %w", err)
	}
	for _, device := range m.Devices {
		if err := ValidatePCIAddress(device.PCI); err != nil {
			return fmt.Errorf("device pci %q: %w", device.PCI, err)
		}
	}
	if m.StopGracePeriodSec < minStopGracePeriodSec {
		return fmt.Errorf("stop_grace_period_sec must be >= %d", minStopGracePeriodSec)
	}
	if err := validateHealthcheck(m.Healthcheck); err != nil {
		return err
	}
	if err := validatePorts(m.Ports, m.Replicas); err != nil {
		return err
	}
	if err := validateMounts(m.Mounts); err != nil {
		return err
	}
	if err := validateWriteFiles(m.CloudInit.WriteFiles); err != nil {
		return err
	}
	return nil
}

func validateInternalNetwork(network *InternalNetworkConfig) error {
	if network == nil {
		return nil
	}
	if err := ValidateMACAddress(network.BaseMAC); err != nil {
		return fmt.Errorf("internal_network.base_mac: %w", err)
	}
	if err := ValidateMACAddress(network.UserBaseMAC); err != nil {
		return fmt.Errorf("internal_network.user_base_mac: %w", err)
	}
	for _, segment := range network.Segments {
		if err := ValidateMACAddress(segment.BaseMAC); err != nil {
			return fmt.Errorf("internal_network.segment %q base_mac: %w", segment.Name, err)
		}
	}
	return nil
}
