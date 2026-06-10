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

## Current Status

No highest-leverage product gaps are currently tracked here.

Recent coverage added:

- Compose network semantics: named internal VM segments, per-service network
  aliases, `dns_search`, explicit service MAC addresses, bridge-backed
  networks, and managed tap-backed networks.
- Device/import coverage: extra qcow2 disks as named volumes, imported NIC
  source/MAC intent, imported USB hostdev metadata, and GPU passthrough
  diagnostics.

The remaining accepted Docker Compose fields such as `network_mode`, `dns`,
`dns_opt`, `links`, and `external_links` stay compatibility metadata unless a
future single-host VM workflow needs different behavior. They are not tracked as
active gaps today.

Add new entries here when a concrete missing feature fits holos' single-host KVM
compose goal and has a clear testable runtime or authoring behavior.

## Explicit Non-Goals

Future missing features should not expand holos into Kubernetes or libvirt. The
project should continue to avoid multi-host clustering, live migration, service
meshes, overlay networks, schedulers, CRDs, and quorum-managed control planes.
