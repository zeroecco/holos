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

Named `networks`, per-service attachments, aliases, `dns_search`, `mac_address`,
explicit bridge-backed networks, and managed tap-backed networks now affect VM
networking. `network_mode`, `dns`, `dns_opt`, `links`, and `external_links` are
still compatibility metadata.

Useful next steps:

- Decide whether Compose `network_mode` should map to additional VM networking
  modes or remain metadata.

This is in scope because predictable VM networking is core to multi-service
stacks. Multi-host overlays, service meshes, and schedulers remain non-goals.

## Hardware And Import Coverage

### Broader Device Import

`holos import` maps vCPU, memory, machine type, host CPU mode, UEFI loader, the
first file disk, extra qcow2 file disks as named volumes, and PCI host devices.
It preserves libvirt NIC source and MAC intent as Compose network metadata, and
preserves USB passthrough vendor/product intent as service device metadata. It
reports warnings for extra disks that cannot be mapped and custom emulators.

## Explicit Non-Goals

The missing features above should not expand holos into Kubernetes or libvirt.
The project should continue to avoid multi-host clustering, live migration,
service meshes, overlay networks, schedulers, CRDs, and quorum-managed control
planes.
