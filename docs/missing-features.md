---
title: Missing Features
description: Product gaps that fit holos' single-host KVM compose goal.
permalink: /missing-features/
---

# Missing Features

holos is designed to make single-host KVM stacks feel as direct as Docker
Compose while keeping VMs as the isolation boundary. The missing features below
fit that goal because they improve authoring, migration from compose files, or
day-to-day operation of local VM stacks without adding a cluster control plane.

## Highest Leverage

### Real Compose Network Semantics

`networks`, per-service network attachments, `network_mode`, `dns`, `dns_opt`,
`dns_search`, `links`, `external_links`, and `mac_address` are accepted today,
but most are compatibility metadata. Holos currently assigns its own internal
VM network and host port forwards.

Useful next steps:

- Map named networks to distinct internal VM segments.
- Honor per-service aliases and DNS search behavior.
- Support explicit service MAC addresses where QEMU can enforce them safely.
- Add a bridge/tap backend for users who need LAN-visible guests.

This is in scope because predictable VM networking is core to multi-service
stacks. Multi-host overlays, service meshes, and schedulers remain non-goals.

## VM Operations

### Snapshots And Volume Lifecycle Commands

Named volumes are qcow2-backed and survive `holos down`. `holos volumes` lists
their path, size, and attachment state, and `holos volumes rm` removes detached
volumes. `holos volumes export` copies detached backing files without
overwriting existing files. There is still no CLI to snapshot or resize volumes.

Useful next steps:

- Consider snapshot commands for root overlays and named volumes.

### Richer Status And Inspection

`holos ps`, logs, and state files expose useful information, but there is no
single inspection command for the resolved manifest, QEMU args, host forwards,
volume paths, health state, and generated SSH endpoint.

Useful next steps:

- Add `holos inspect <project|instance>`.
- Include JSON output for automation.
- Keep sensitive generated key material out of output.

### Image Lockfile Workflow

Docs recommend keeping private image checksums next to a project, but holos
does not generate or enforce a project image lockfile.

Useful next steps:

- Add `holos images lock -f holos.yaml`.
- Verify local/private image paths against the lock before `up`.
- Record image size, format, digest, and resolved path.

## Hardware And Import Coverage

### Broader Device Import

`holos import` maps vCPU, memory, machine type, host CPU mode, UEFI loader, the
first file disk, and PCI host devices. It reports warnings for extra disks,
bridged NICs, USB passthrough, and custom emulators.

Useful next steps:

- Convert additional file disks into named volumes when possible.
- Map bridged NIC intent into a future bridge/tap network backend.
- Represent USB passthrough if QEMU support is added to the VM schema.

### GPU Convenience

PCI passthrough is supported, but users still need to discover IOMMU groups and
bind devices to `vfio-pci` themselves.

Useful next steps:

- Add guided diagnostics for GPU pairs, drivers, and IOMMU group safety.
- Emit suggested host setup commands without applying them automatically.
- Detect common NVIDIA ROM and UEFI pitfalls before launch.

## Explicit Non-Goals

The missing features above should not expand holos into Kubernetes or libvirt.
The project should continue to avoid multi-host clustering, live migration,
service meshes, overlay networks, schedulers, CRDs, and quorum-managed control
planes.
