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
git tag -a v0.2.1 -m "v0.2.1"
git push origin v0.2.1
```

The workflow cross-compiles Linux and macOS binaries for amd64 and arm64,
packages them with `LICENSE`, `NOTICE`, and `README.md`, computes SHA-256
checksums, and publishes a GitHub release.

To rehearse locally without publishing:

```bash
goreleaser release --snapshot --clean --skip=publish
```
