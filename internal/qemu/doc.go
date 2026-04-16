// Package qemu builds command-line arguments for qemu-system-x86_64.
//
// [BuildArgs] takes a service [config.Manifest] and a per-launch [LaunchSpec]
// and produces a complete argv slice. The generated command line includes:
//
//   - KVM acceleration on a q35 (or configured) machine.
//   - A virtio root disk backed by a qcow2 overlay.
//   - A user-mode NIC with TCP host-forward rules for each port mapping.
//   - An optional socket multicast NIC for inter-VM L2 networking.
//   - A serial console on a Unix socket (with log file) and a QMP socket.
//   - UEFI pflash drives when OVMF firmware paths are provided.
//   - 9p virtfs mounts for host directory sharing.
//   - VFIO PCI passthrough devices (GPUs, NICs, etc.).
//   - Any extra user-supplied QEMU arguments appended at the end.
//
// When PCI devices are present, kernel-irqchip=on is added to the machine
// options for NVIDIA GPU compatibility.
package qemu
