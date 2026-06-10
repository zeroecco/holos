package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultAPIVersion         = "holos/v1alpha1"
	DefaultKind               = "Service"
	DefaultReplicas           = 1
	DefaultVCPU               = 1
	DefaultMemoryMB           = 512
	DefaultMachine            = "q35"
	DefaultCPUModel           = "host"
	DefaultNetworkMode        = "user"
	DefaultUser               = "ubuntu"
	DefaultProtocol           = ProtocolTCP
	ProtocolTCP               = "tcp"
	ProtocolUDP               = "udp"
	ImageFormatQCOW2          = "qcow2"
	ImageFormatRaw            = "raw"
	DefaultFilePermissions    = "0644"
	DefaultFileOwner          = "root:root"
	DefaultStopGracePeriodSec = 30

	DefaultHealthIntervalSec = 30
	DefaultHealthRetries     = 3
	DefaultHealthTimeoutSec  = 5

	// minVolumeSizeBytes is enforced in Validate so a corrupted or
	// hand-written manifest can't request a 0-byte volume that would
	// confuse qemu-img.
	minVolumeSizeBytes = 1 << 20 // 1 MiB
)

const (
	ImageOSSystemd = "systemd"
	ImageOSOpenRC  = "openrc"
)

// Mount kind discriminators. Left as strings (not iota) so the on-disk
// JSON is self-documenting and forward-compatible with future kinds.
const (
	MountKindBind   = "bind"
	MountKindVolume = "volume"
)

// LoadManifest reads a JSON manifest file, applies defaults, resolves
// relative paths against the manifest's directory, and validates the result.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	manifest.applyDefaults()
	if err := manifest.resolvePaths(filepath.Dir(path)); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
