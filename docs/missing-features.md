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

### Guest Distribution For `configs` And `secrets`

Top-level `configs` and `secrets` plus per-service references are parsed for
Compose compatibility, but they are not materialized inside guests. Users must
currently model those files with `cloud_init.write_files`.

Useful next steps:

- Resolve config and secret references into generated cloud-init files.
- Preserve ownership, mode, and target path semantics where Compose provides
  them.
- Keep secret file permissions restrictive in the state directory and generated
  seed.

This is in scope because VM stacks need repeatable configuration injection.

### Runtime Lifecycle Hooks

`post_start` is translated into first-boot commands, but `pre_stop` cannot run
at shutdown time because holos does not have a VM shutdown hook. The current
behavior is useful metadata, not full lifecycle support.

Useful next steps:

- Execute `pre_stop` over SSH before ACPI shutdown when the guest is reachable.
- Surface hook failure policy clearly.
- Record hook output in instance logs or state for debugging.

This is in scope because lifecycle hooks are operational behavior for a single
VM, not orchestration scope creep.

## Compose Compatibility Gaps

### UDP Port Forwarding

Port declarations accept only TCP. UDP mappings are rejected, and QEMU argument
generation only supports TCP host forwarding.

Useful next steps:

- Extend port parsing and validation to accept UDP.
- Render QEMU `hostfwd=udp:` entries.
- Add tests for mixed TCP/UDP services and replica port increments.

### Healthcheck `start_interval`

Compose `start_interval` is accepted, but holos probes using `interval`.

Useful next steps:

- Use `start_interval` during `start_period`.
- Fall back to `interval` when `start_interval` is omitted.
- Persist the resolved values in runtime records for inspectability.

### More Dockerfile Instruction Coverage

The Dockerfile translator supports `FROM`, `RUN`, `COPY`, `ENV`, and `WORKDIR`.
Unsupported instructions fail loudly. Some common instructions have clear
Holos equivalents, but users must currently rewrite them into compose or
cloud-init fields.

Useful next steps:

- Translate `EXPOSE` into suggested or optional port metadata.
- Translate `HEALTHCHECK` into service `healthcheck`.
- Consider limited `ADD` support for local files while preserving build-context
  escape checks.

Dockerfile `CMD` and `ENTRYPOINT` are better represented by service
`command`/`entrypoint`, which holos already supports.

## VM Operations

### Snapshots And Volume Lifecycle Commands

Named volumes are qcow2-backed and survive `holos down`, but there is no CLI to
list, snapshot, resize, export, or remove them safely.

Useful next steps:

- Add `holos volumes` listing with project, size, path, and attachment state.
- Add explicit remove/export commands that refuse active attachments.
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
