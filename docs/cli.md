---
title: CLI Guide
description: holos command-line workflows for VMs, SSH, systemd, imports, and diagnostics.
permalink: /cli/
---

# CLI Guide

## Ad Hoc VMs

`holos run` launches a one-off VM without a compose file:

```bash
holos run ubuntu:noble
holos run --vcpu 4 --memory 4G ubuntu:noble
holos run -p 8080:80 --pkg nginx \
  --runcmd 'systemctl enable --now nginx' alpine
holos run -v ./code:/srv:ro ubuntu:noble
holos run --device 0000:01:00.0 ubuntu:noble
holos run --image-os openrc ./custom-alpine.qcow2
holos run alpine -- echo hello world
```

The synthesized compose file is stored at
`state_dir/runs/<name>/holos.yaml`. The project name is derived from the image
unless you pass `--name`.

Follow-up commands use the project name printed by `holos run`:

```bash
holos exec <name>
holos console <name>
holos logs <name>
holos down <name>
```

`holos run` exits once the VM is started. VMs are always detached. Use
`holos exec` for an interactive shell and `holos console` for serial boot logs.

## SSH With `holos exec`

Every `holos up` creates a project SSH key under `state_dir/ssh/<project>/` and
injects the public key with cloud-init. Each instance gets a host port forwarded
to guest port 22.

```bash
holos exec web-0
holos exec db-0 -- pg_isready
holos exec my-project -- uname -a
```

`-u <user>` overrides the login user. Otherwise holos uses the service's
resolved `cloud_init.user`, then the image convention (`debian`, `alpine`,
`fedora`, `arch`, `ubuntu`), then `ubuntu`.

On a fresh VM, `holos exec` waits up to 60s for sshd to be ready. Use `-w 0` to
disable that wait or `-w 5m` for slow first boots.

## Project Locks

Lifecycle commands take a per-project lock before reading or writing runtime
state. `holos up`, `run`, `down`, `start`, `stop`, and `ps -f` wait up to
`--lock-timeout` (default `5m`) when another holos process is already operating
on the same project. Use `--no-wait` when automation should fail fast instead
of waiting:

```bash
holos up --lock-timeout 30s
holos down --no-wait demo
```

Lock errors include the lock path and last recorded holder metadata when
available.

## Image Verification

Built-in images carry checksum metadata. `holos pull`, `holos up`, and
`holos run` verify downloads before promoting them into the cache and re-check a
cached file before reuse. To audit the cache explicitly:

```bash
holos verify alpine
holos verify --all
```

`holos images` shows the guest OS metadata and hash algorithm used for each
built-in entry. Local image paths are not trusted by name; set `image_format`
and `image_os` in compose, or `--image-os` with `holos run`, when a custom image
does not match the defaults. For private qcow2 images, keep a project checksum
manifest such as `images/SHA256SUMS` or `holos.images.lock` and verify it before
launching; the [threat model](./threat-model.md) has a fuller checklist.

## Reboot Survival

Install a systemd unit so a project comes back after host reboot:

```bash
holos install --enable
holos install --system --enable
holos install --dry-run
holos uninstall
```

User units go under `~/.config/systemd/user/holos-<project>.service`; system
units go under `/etc/systemd/system/`.

If you use `holos install --system --user <name>`, also pass an explicit
`--state-dir` that the target user owns. holos keeps state directories
owner-only.

## Import From virsh

`holos import` reads libvirt domain XML and emits a starting `holos.yaml`:

```bash
holos import web-prod db-prod
holos import --all -o holos.yaml
holos import --xml ./web.xml
holos import --connect qemu:///system api
```

It maps vCPU, memory, machine type, host CPU mode, UEFI loader, the first file
disk, and PCI host devices. Extra disks, bridged NICs, USB passthrough, and
custom emulators are reported as warnings so you can edit the generated compose
file before `holos up`.

## Doctor

`holos doctor` checks the host without launching a VM:

```bash
holos doctor
holos doctor --json
```

It verifies Linux/KVM availability, QEMU tools, a cloud-init seed builder, SSH,
optional OVMF firmware for UEFI and PCI passthrough, and state-dir writability.
