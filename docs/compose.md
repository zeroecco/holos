---
title: Compose File
description: The holos.yaml format for defining KVM VM stacks.
permalink: /compose/
---

# Compose File

`holos.yaml` is intentionally close to docker-compose, but the unit of work is a
VM. Each service becomes one or more QEMU instances with a qcow2 overlay,
cloud-init seed, generated SSH access, and optional named volumes.

For editor completion and validation, use the schema at
[`docs/holos.schema.json`](./holos.schema.json).

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

Core fields:

- `services`: map of service name to VM definition.
- `image`: image alias (`alpine`, `ubuntu:noble`) or local image path.
- `image_os`: optional guest OS family for local/custom images (`systemd` or
  `openrc`). Built-in images set this metadata automatically.
- `dockerfile`: Dockerfile translated into cloud-init provisioning.
- `replicas`: number of instances for the service. Host ports auto-increment by
  replica index.
- `vm`: virtual hardware (`vcpu`, `memory_mb`, `machine`, `cpu_model`, `uefi`,
  `extra_args`).
- `ports`: TCP forwards. Use `"host:guest"` for a fixed host port,
  `"guest"` to have holos allocate an ephemeral host port, or append
  `"/tcp"` explicitly (`"8080:80/tcp"`).
- `volumes`: bind mounts or top-level named volumes with `SRC:TGT[:ro|rw]`.
- `depends_on`: service startup ordering. If the dependency has a healthcheck,
  dependents wait until it is healthy.
- `cloud_init`: user, packages, write files, boot commands, and run commands.
- `stop_grace_period`: ACPI shutdown wait before SIGTERM/SIGKILL.
- `healthcheck`: SSH-based readiness probe.

## Graceful Shutdown

`holos stop` and `holos down` send QMP `system_powerdown` to the guest, then
wait up to `stop_grace_period` for QEMU to exit on its own. If the guest does
not halt in time, or QMP is unreachable, the runtime falls back to SIGTERM then
SIGKILL.

```yaml
services:
  db:
    image: ubuntu:noble
    stop_grace_period: 60s
```

## Data Volumes

Top-level `volumes:` declares named data stores under
`state_dir/volumes/<project>/<name>.qcow2`. They survive `holos down`; teardown
only removes per-instance symlinks.

```yaml
name: demo

services:
  db:
    image: ubuntu:noble
    volumes:
      - pgdata:/var/lib/postgresql
      - snapshots:/mnt/snapshots:ro

volumes:
  pgdata:
    size: 20G
  snapshots:
    size: 50G
```

Named volumes attach as virtio-blk devices with stable serials like
`vol-pgdata`, so the guest sees `/dev/disk/by-id/virtio-vol-pgdata`. For
read-write volumes, cloud-init creates an ext4 filesystem and fstab entry. For
read-only volumes, holos skips formatting and writes `ro,nofail` to fstab. If a
guest mount fails, the cloud-init command exits non-zero and writes a
`holos: failed to mount volume ...` error to the instance console log.

## Healthchecks And `depends_on`

A service with a healthcheck blocks dependents from starting until the probe
passes. The probe runs over SSH using the same generated key as `holos exec`.

```yaml
services:
  db:
    image: postgres-cloud.qcow2
    healthcheck:
      test: ["pg_isready", "-U", "postgres"]
      interval: 2s
      retries: 30
      start_period: 10s
      timeout: 3s

  api:
    image: api.qcow2
    depends_on: [db]
```

`test` accepts a list (exec form) or a string (wrapped as `sh -c`). Failures
during `start_period` do not consume retry budget. Set
`HOLOS_HEALTH_BYPASS=1` to skip the actual probe in CI environments that cannot
SSH into guests.

## Networking

Every service can reach every other service by name:

- Each VM gets a user-mode NIC for host port forwarding and a socket multicast
  NIC for inter-VM traffic.
- Static IPs are assigned on the internal `10.10.0.0/24` segment.
- `/etc/hosts` is populated by cloud-init so `db`, `web-0`, and `web-1` resolve.
- No libvirt bridge or root-owned network setup is required.

## GPU And PCI Passthrough

Pass physical GPUs or other PCI devices directly to a VM via VFIO:

```yaml
services:
  ml:
    image: ubuntu:noble
    vm:
      vcpu: 8
      memory_mb: 16384
    devices:
      - pci: "01:00.0"
      - pci: "01:00.1"
    ports:
      - "8888:8888"
```

holos enables UEFI automatically when devices are present, copies OVMF vars per
instance, sets NVIDIA-friendly machine options, and accepts optional `rom_file`
paths for custom VBIOS ROMs. You still need host IOMMU setup and the relevant
devices bound to `vfio-pci`.

## Images

Use built-in image aliases or local paths:

```yaml
services:
  web:
    image: alpine
  api:
    image: ubuntu:noble
  db:
    image: ./images/db.qcow2
    image_format: qcow2
```

Available aliases include `alpine`, `arch`, `debian`, `ubuntu`, and `fedora`.
Run `holos images` to see tags and defaults.

## Dockerfile Provisioning

A Dockerfile can provision a VM when you want familiar build steps without
building a container image. holos translates supported instructions into
cloud-init:

```yaml
services:
  api:
    dockerfile: ./Dockerfile
    ports:
      - "3000:3000"
```

Supported instructions are `FROM`, `RUN`, `COPY`, `ENV`, and `WORKDIR`.
Unsupported instructions fail loudly. For example, use `services.<name>.ports`
instead of `EXPOSE`, `services.<name>.healthcheck` instead of `HEALTHCHECK`, and
guest systemd units or `cloud_init.runcmd` instead of `CMD` / `ENTRYPOINT`.

When `image` is omitted, the base image is taken from the Dockerfile's `FROM`
line. Dockerfile instructions run before `cloud_init.runcmd`.

`COPY` sources are resolved relative to the Dockerfile directory, must stay
inside the build context, and must be files. Use volumes for directories.

## Extra QEMU Arguments

Pass arbitrary flags through with `vm.extra_args`:

```yaml
services:
  gpu:
    image: ubuntu:noble
    vm:
      extra_args:
        - "-device"
        - "virtio-gpu-pci"
        - "-display"
        - "egl-headless"
```

Arguments are appended after holos-managed flags. holos does not validate them.

## Defaults

- `replicas`: `1`
- `vm.vcpu`: `1`
- `vm.memory_mb`: `512`
- `vm.machine`: `q35`
- `vm.cpu_model`: `host`
- `cloud_init.user`: image-specific default, then `ubuntu`
- `image_format`: inferred from extension or registry metadata
