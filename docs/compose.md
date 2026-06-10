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

- Top-level `version` and `include`: accepted for Docker Compose compatibility.
  `include` accepts short and long syntax. Existing include files are loaded
  and merged before the main file is resolved; definitions in the main file
  take precedence. Included service paths resolve relative to the included
  file's `project_directory` when set, otherwise the included file directory.
- `x-*` extension fields: accepted and ignored anywhere in the Compose file
  while preserving strict typo checks for non-extension fields.
- Compose merge-control tags `!reset` and `!override`: accepted during load.
  Holos normalizes them before strict decoding.
- `services`: map of service name to VM definition.
- `image`: image alias (`alpine`, `ubuntu:noble`) or local image path.
- `image_os`: optional guest OS family for local/custom images (`systemd` or
  `openrc`). Built-in images set this metadata automatically.
- `build`, `dockerfile`: Docker Compose build syntax and Holos Dockerfile
  syntax translated into cloud-init provisioning.
- `command`, `entrypoint`: Docker Compose command fields. In Holos these are
  translated into first-boot `cloud_init.runcmd` entries after Dockerfile
  provisioning and before explicit `cloud_init.runcmd`.
- `working_dir`: applies to the generated `command`/`entrypoint` runcmd by
  prefixing it with `cd`.
- `user`: Docker Compose user field. Holos treats this as the cloud-init user;
  `cloud_init.user` takes precedence when set.
- `container_name`, `platform`, `pull_policy`, `pull_refresh_after`,
  `profiles`, `restart`, `stop_signal`, `oom_kill_disable`, `pids_limit`:
  accepted for Docker Compose compatibility. They are currently metadata/no-op
  fields in Holos VM execution.
- `deploy`: accepts Docker Compose deploy syntax. `deploy.replicas` maps to
  Holos replicas, and `deploy.resources.limits` can provide vCPU/memory
  fallbacks. Deploy device reservations and other Swarm-specific deploy fields
  are accepted as compatibility metadata.
- `replicas`: number of instances for the service. Docker Compose `scale` is
  also accepted as an alias, as is `deploy.replicas`. Host ports auto-increment
  by replica index.
- `hostname`, `domainname`: Docker Compose naming fields. Holos writes these
  into cloud-init as the guest hostname; `cloud_init.hostname` takes precedence
  when set.
- `cpus`, `mem_limit`: Docker Compose resource fields mapped to Holos
  `vm.vcpu` and `vm.memory_mb` when those VM fields are omitted. Compose
  `deploy.resources.limits.cpus` and `deploy.resources.limits.memory` are also
  used as fallbacks. Fractional CPU values round up to whole vCPUs.
- `init`, `privileged`, `read_only`, `tty`, `stdin_open`: accepted for Docker
  Compose compatibility. They are no-ops in Holos because each service is a VM
  with its own init process, isolation boundary, disk policy, and console.
- `cap_add`, `cap_drop`, `cgroup`, `cgroup_parent`, `cpu_count`,
  `cpu_percent`, `cpu_period`, `cpu_quota`, `cpu_rt_period`,
  `cpu_rt_runtime`, `cpu_shares`, `cpuset`, `credential_spec`, `isolation`,
  `ipc`, `pid`,
  `mem_reservation`, `mem_swappiness`, `memswap_limit`, `oom_score_adj`,
  `runtime`, `security_opt`, `shm_size`, `storage_opt`, `sysctls`, `tmpfs`,
  `ulimits`, `uts`, `userns_mode`, `blkio_config`, `device_cgroup_rules`,
  `device_read_bps`, `device_read_iops`, `device_write_bps`,
  `device_write_iops`: accepted for Docker Compose compatibility. They are
  currently metadata/no-op fields in Holos VM execution.
- `vm`: virtual hardware (`vcpu`, `memory_mb`, `machine`, `cpu_model`, `uefi`,
  `extra_args`).
- `ports`: TCP and UDP forwards. Use Docker Compose short syntax like
  `"host:guest"`, `"guest"`, `"host_ip:host:guest"`, or
  `"host_ip:host:guest_ip:guest"`. Append `"/tcp"` or `"/udp"` explicitly
  when desired (`"8080:80/tcp"`, `"5353:5353/udp"`). Docker Compose long
  syntax is also accepted with
  `target`, `published`, `host_ip`, `protocol`, `app_protocol`, `mode`, and
  `name`. Equal-length short-form ranges such as `"8080-8081:80-81"` expand
  to multiple forwards; long-form `published` ranges map each host port to the
  same target. Holos supports TCP, UDP, and IPv4 bind/guest addresses. Host
  ports bind to `127.0.0.1` unless a host IP is provided.
- `volumes`: bind mounts or top-level named volumes with `SRC:TGT[:ro|rw]`.
  Docker Compose long syntax is also accepted for `type: bind` and
  `type: volume`; `tmpfs`, `image`, `npipe`, and `cluster` entries are accepted
  as compatibility no-ops.
- `devices`: Holos PCI passthrough objects continue to map to VM devices.
  Docker Compose string/CDI device entries are accepted as compatibility
  metadata.
- `depends_on`: service startup ordering. Accepts Docker Compose list syntax
  (`[db]`) and long mapping syntax with `condition`, `restart`, and
  `required`. If the dependency has a healthcheck, dependents wait until it is
  healthy.
- `labels`: metadata copied into the resolved service manifest. Accepts Docker
  Compose map syntax and list syntax (`key=value` or `key` for an empty value).
- `annotations`, `attach`, `dns`, `dns_opt`, `dns_search`, `develop`,
  `extends`, `expose`, `external_links`, `gpus`, `group_add`, `links`,
  `logging`, `mac_address`, `network_mode`, `post_start`, `pre_stop`,
  `provider`, `use_api_socket`, `volumes_from`: accepted for Docker Compose
  compatibility. `post_start` commands are translated into first-boot
  `cloud_init.runcmd`; `pre_stop` commands run over SSH before ACPI shutdown
  when `holos stop`, `holos down`, scale-down, or removed-service cleanup stops
  a running VM. If a configured `pre_stop` command cannot run or exits non-zero,
  the stop operation fails before powerdown so the failure is visible. The
  remaining fields in this list are currently metadata/no-op fields in Holos VM
  execution.
- `label_file`: loads label files relative to the compose file. Inline
  `labels` override file-provided labels.
- `extra_hosts`: additional guest host mappings. Accepts Docker Compose map
  syntax and list syntax (`host=ip` or `host:ip`).
- `environment`: guest-wide environment variables written to `/etc/environment`.
  Accepts Docker Compose map syntax and list syntax; unset entries are omitted.
- `env_file`: environment files loaded relative to the compose file. Accepts a
  string, a list of strings, and Docker Compose mapping entries with `path`,
  `required`, and `format`; inline `environment` values take precedence.
  `format: raw` is supported, and other formats are rejected.
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
only removes per-instance symlinks. Docker Compose volume metadata fields such
as `name`, `driver`, `driver_opts`, `external`, and `labels` are accepted for
compatibility; Holos uses `size` for the qcow2 backing file.

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

Top-level `networks` and per-service `networks` are accepted for Docker
Compose compatibility. Holos currently uses its own internal VM network plan,
so these declarations do not alter runtime networking.

Top-level `configs`/`secrets` and per-service references are distributed into
guests as cloud-init write files. `file`, `content` (configs), and
`environment` sources are supported. Service-level `target`, `uid`, `gid`, and
`mode` control the generated file path, owner, and permissions. Configs default
to `0444`; secrets default to `0400` under `/run/secrets/<name>`. External
configs and secrets are rejected when referenced because holos has no external
secrets backend. Config `template_driver` is accepted as metadata.

Top-level `models` and per-service `models` references are accepted for Docker
Compose compatibility, including `model`, `context_size`, and `runtime_flags`.
They are metadata only in Holos today.

Named volumes attach as virtio-blk devices with stable serials like
`vol-pgdata`, so the guest sees `/dev/disk/by-id/virtio-vol-pgdata`. For
read-write volumes, cloud-init creates an ext4 filesystem and fstab entry. For
read-only volumes, holos skips formatting and writes `ro,nofail` to fstab. If a
guest mount fails, the cloud-init command exits non-zero and writes a
`holos: failed to mount volume ...` error to the instance console log.

Use `holos volumes` to list named volume backing files, declared size, path, and
the instances that currently reference each volume. Add `--json` for structured
output, or `-f holos.yaml` to limit the list to one project. Use
`holos volumes rm <project> <volume>` to remove a detached named volume; removal
fails while any instance workdir still references the volume. Use
`holos volumes export <project> <volume> <path>` to copy a detached volume
backing file to a new destination without overwriting existing files.

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

`test` accepts a list (exec form) or a string (wrapped as `sh -c`). Docker
Compose's `test: ["NONE"]` and `disable: true` forms disable the healthcheck.
`start_interval` controls the probe cadence during `start_period`; when omitted,
Holos uses `interval` in both phases. Failures during `start_period` do not
consume retry budget. Set `HOLOS_HEALTH_BYPASS=1` to skip the actual probe in CI
environments that cannot SSH into guests.

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

Available aliases include `alpine`, `arch`, `debian`, `ubuntu`, `fedora`,
`almalinux`, `rocky`, and `centos-stream`. Run `holos images` to see tags and
defaults.

For local or private qcow2 images, holos treats the file as an
operator-supplied artifact. Set `image_format` and `image_os` explicitly, verify
the image checksum before `holos up`, and consider keeping an
`images/SHA256SUMS` or `holos.images.lock` file next to `holos.yaml` so teams
can review exactly which private image build a project was tested against. See
the [threat model](./threat-model.md) for verification and lockfile guidance.
Use `holos images lock -f holos.yaml` to generate `holos.images.lock` from the
resolved service images.

## Dockerfile Provisioning

A Dockerfile can provision a VM when you want familiar build steps without
building a container image. holos translates supported instructions into
cloud-init:

```yaml
services:
  api:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "3000:3000"
```

`build` accepts Docker Compose string syntax (`build: ./app`) and mapping
syntax with `context` and `dockerfile`. Holos accepts common Compose build
metadata such as `args`, `cache_from`, `extra_hosts`, `isolation`, `labels`,
`no_cache`, `pull`, `provenance`, `sbom`, `shm_size`, `ssh`, `tags`, `target`,
`ulimits`, and `platforms`, but only `context`, `dockerfile`, and
`dockerfile_inline` affect provisioning today. `additional_contexts` accepts
both map syntax and `NAME=VALUE` list syntax.

Supported instructions are `FROM`, `RUN`, `COPY`, local-file `ADD`, `ENV`,
`WORKDIR`, `EXPOSE`, and `HEALTHCHECK`.
Unsupported instructions fail loudly. For example, use guest systemd units or
`cloud_init.runcmd` instead of `CMD` / `ENTRYPOINT`.

When `image` is omitted, the base image is taken from the Dockerfile's `FROM`
line. Dockerfile instructions run before `cloud_init.runcmd`.

`HEALTHCHECK` maps to the service healthcheck when the compose service does not
define one explicitly. `HEALTHCHECK NONE` disables the Dockerfile-provided
healthcheck. `EXPOSE` maps to guest-only port metadata when the compose service
does not define `ports`; explicit compose ports override Dockerfile exposure.
`COPY` and `ADD` sources are resolved relative to the build context for `build`,
or the Dockerfile directory for `dockerfile`. They must stay inside the context
and must be files. `ADD` remote URLs and automatic archive extraction are not
supported; use `RUN` or `cloud_init.runcmd` for downloads and extraction. Use
volumes for directories.

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
