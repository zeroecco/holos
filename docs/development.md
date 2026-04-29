---
title: Development
description: Build, test, host requirements, and release process for holos.
permalink: /development/
---

# Development

## Build

```bash
go build -o bin/holos ./cmd/holos
go test ./...
```

Build a guest image with mkosi:

```bash
./scripts/build-image.sh
```

## Host Requirements

Runtime commands need a Linux KVM host with:

- `/dev/kvm`
- `qemu-system-x86_64`
- `qemu-img`
- One of `cloud-localds`, `genisoimage`, `mkisofs`, or `xorriso`
- OVMF / edk2-ovmf firmware for UEFI and PCI passthrough

macOS builds are shipped for offline work: `validate`, `import`, `images`,
`pull`, and compose-file authoring.

## Cutting A Release

Releases are produced by GoReleaser on every `v*` tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

The workflow cross-compiles Linux and macOS binaries for amd64 and arm64,
packages them with `LICENSE`, `NOTICE`, and `README.md`, computes SHA-256
checksums, publishes a GitHub release, and emits GitHub artifact attestations.
Release notes keep the checksum file and signed provenance verification command
visible near the install command so operators can verify downloads before
extracting them.

To rehearse locally without publishing:

```bash
goreleaser release --snapshot --clean --skip=publish
```
