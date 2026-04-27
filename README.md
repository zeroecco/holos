<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./docs/holos-lockup-dark.svg">
  <img alt="holos" src="./docs/holos-lockup-light.svg" width="340">
</picture>

Docker compose for KVM. Define multi-VM stacks in one YAML file. No libvirt, no
XML, no distributed control plane.

Website and docs: <https://zeroecco.github.io/holos/>

The primitive is a VM, not a container. Every workload instance gets its own
kernel boundary, qcow2 overlay, cloud-init seed, and generated SSH access.

## Quick Start

> Requires Linux + `/dev/kvm`. macOS builds run offline commands like
> `validate`, `import`, `images`, and `pull`, but `up` and `run` need a KVM
> host.

One disposable VM, no compose file:

```bash
holos run alpine
holos exec <printed-project-name>
holos down <printed-project-name>
```

A single-service stack you can `curl`. Save as `holos.yaml`:

```yaml
name: hello

services:
  web:
    image: ubuntu:noble
    ports:
      - "8080:80"
    cloud_init:
      packages:
        - nginx
      write_files:
        - path: /var/www/html/index.html
          content: "hello from holos\n"
      runcmd:
        - systemctl restart nginx
```

```bash
holos up
curl localhost:8080
holos down hello
```

That is a real VM booting a cloud image, installing a package, writing config,
and forwarding a host port.

## Install

Pre-built binaries are attached to every
[GitHub release](https://github.com/zeroecco/holos/releases):

```bash
TAG=v0.2.2
curl -L https://github.com/zeroecco/holos/releases/download/$TAG/holos_${TAG#v}_Linux_x86_64.tar.gz \
  | sudo tar -xz -C /usr/local/bin holos
holos version
holos doctor
```

Or build from source:

```bash
go build -o bin/holos ./cmd/holos
go test ./...
bin/holos doctor
```

## CLI

```text
holos up [-f holos.yaml]             start all services
holos run [flags] <image> [-- cmd...] launch a one-off VM
holos down <project>                 stop and remove a project
holos ps [-f holos.yaml]             list running projects
holos start [-f holos.yaml] [svc]    start a stopped service or all services
holos stop [-f holos.yaml] [svc]     stop a service or all services
holos console <project> [<inst>]     attach serial console
holos exec <project> [<inst>] [-- cmd...]
                                     SSH into an instance
holos logs <project> [<svc|inst>]    show console logs
holos validate [-f holos.yaml]       validate compose file
holos pull <image>                   pull a cloud image
holos images                         list available images
holos devices [--gpu]                list PCI devices and IOMMU groups
holos doctor [--json]                check host dependencies
holos install [-f holos.yaml] [--system] [--enable]
                                     install a systemd unit
holos uninstall [-f holos.yaml] [--system]
                                     remove the systemd unit
holos import [vm...] [--all] [--xml file] [--connect uri] [-o file]
                                     convert virsh VMs into holos.yaml
```

## Docs

- [Website](https://zeroecco.github.io/holos/): landing page and rendered docs.
- [CLI guide](./docs/cli.md): ad hoc VMs, `exec`, systemd install, virsh import,
  and `doctor`.
- [Compose file](./docs/compose.md): services, volumes, healthchecks,
  networking, PCI passthrough, Dockerfile provisioning, and defaults.
- [JSON Schema](./docs/holos.schema.json): editor completion and validation for
  `holos.yaml`.
- [Examples](./examples/README.md): runnable and template stacks with
  README-style explanations.
- [Development](./docs/development.md): build, test, host requirements, and
  release process.
- [Security policy](./SECURITY.md): supported versions and private reporting.
- [Contributing](./CONTRIBUTING.md): build, test, style, and PR conventions.

## Examples

Start with the small nginx example:

```bash
holos up -f examples/alpine-nginx/holos.yaml
curl localhost:8080
holos down alpine-nginx
```

The examples directory also includes Dockerfile provisioning, GPU passthrough,
and a multi-service stack that shows `depends_on`, generated config, and
replicas.

## Host Requirements

- Linux with `/dev/kvm`
- `qemu-system-x86_64`
- `qemu-img`
- One of `cloud-localds`, `genisoimage`, `mkisofs`, or `xorriso`
- OVMF / edk2-ovmf firmware for UEFI or PCI passthrough
- `ssh` for `holos exec` and healthchecks

Run `holos doctor` to check the host.

## Troubleshooting

### SSH resets on first boot

`kex_exchange_identification: read: Connection reset by peer` usually means
cloud-init is still regenerating host keys and restarting sshd. `holos exec`
waits up to 60s by default, but very slow first boots may need another retry or
`holos exec -w 5m <project>`.

### Console shows `Login incorrect`

The serial console may attempt autologin before cloud-init creates the user.
Wait for `cloud-init ... finished` in the console log, then use `holos exec`.
Cloud images generally do not ship with a console password, and holos does not
add one.

### `up` fails on macOS

KVM is a Linux kernel feature. macOS binaries are useful for authoring and
offline commands, but `holos up` and `holos run` must execute on a Linux KVM
host.

## Non-Goals

holos is not Kubernetes. It does not try to solve multi-host clustering, live
migration, service meshes, overlay networks, schedulers, CRDs, or control plane
quorum.

The goal is to make KVM workable for single-host stacks without importing the
operational shape of Kubernetes.

## License

Licensed under the [Apache License, Version 2.0](./LICENSE). See
[`NOTICE`](./NOTICE) for attribution.
