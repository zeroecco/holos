package qemu

// PortMapping is a resolved host-to-guest TCP port forward assigned to a
// running instance.
type PortMapping struct {
	Name      string `json:"name"`
	HostAddr  string `json:"host_addr,omitempty"`
	HostPort  int    `json:"host_port"`
	GuestAddr string `json:"guest_addr,omitempty"`
	GuestPort int    `json:"guest_port"`
	Protocol  string `json:"protocol"`
}

// LaunchSpec carries the per-instance paths and port mappings needed to
// construct QEMU arguments. The runtime populates this after creating the
// overlay, seed image, and allocating ports.
type LaunchSpec struct {
	Name        string
	Index       int
	OverlayPath string
	SeedPath    string
	LogPath     string
	SerialPath  string
	QMPPath     string
	Ports       []PortMapping
	OVMFCode    string // path to OVMF_CODE.fd (read-only, shared)
	OVMFVars    string // path to per-instance OVMF_VARS.fd copy (writable)
	// SSHPort is the host-side TCP port that should forward to the
	// guest's sshd (22/tcp). The runtime allocates it on first boot to
	// back `holos exec`. Zero means no ssh forward was requested; the
	// user-mode netdev is built without the sshd hostfwd so we don't
	// occupy ports unnecessarily when the feature is disabled.
	SSHPort int
	// TapIfNames maps QEMU netdev IDs (net1, net2, ...) to host tap
	// interfaces that the runtime created before launch.
	TapIfNames map[string]string
	// Volumes are resolved named-volume attachments for this instance.
	// Each entry becomes a virtio-blk block device exposed to the guest
	// with a stable serial so udev creates /dev/disk/by-id/virtio-<serial>.
	Volumes []VolumeAttachment
}

// VolumeAttachment is a single qcow2-backed block device attached to an
// instance, produced by the runtime when materialising named volumes.
type VolumeAttachment struct {
	// Name is the logical volume name from the compose file (e.g. "data").
	Name string
	// DiskPath is the host-visible path the guest should open. The
	// runtime points this at a workdir symlink that targets the
	// project-level qcow2 file, so tearing the workdir down never
	// removes the volume data.
	DiskPath string
	// ReadOnly maps the compose `:ro` suffix on a named volume to
	// QEMU's drive readonly=on. Without this the runtime silently
	// dropped the flag; operators who asked for a read-only volume
	// still got a writable drive and could corrupt shared data.
	ReadOnly bool
}
