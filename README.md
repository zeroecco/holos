# holos

Docker compose for KVM. Define multi-VM stacks in a single YAML file. No libvirt, no XML, no distributed control plane.

The primitive is a VM, not a container. Every workload instance gets its own kernel boundary, its own qcow2 overlay, and its own cloud-init seed.

## Quick Start

Write a `holos.yaml`:

```yaml
name: my-stack

services:
  db:
    image: ubuntu:noble
    vm:
      vcpu: 2
      memory_mb: 1024
    cloud_init:
      packages:
        - postgresql
      runcmd:
        - systemctl enable postgresql
        - systemctl start postgresql

  web:
    image: ubuntu:noble
    replicas: 2
    depends_on:
      - db
    ports:
      - "8080:80"
    volumes:
      - ./www:/srv/www:ro
    cloud_init:
      packages:
        - nginx
      write_files:
        - path: /etc/nginx/sites-enabled/default
          content: |
            server {
                listen 80;
                location / { proxy_pass http://db:5432; }
            }
      runcmd:
        - systemctl restart nginx
```

Bring it up:

```bash
holos up
```

That's it. Two nginx VMs and a postgres VM, all on the same host, all talking to each other by name.

## CLI

```
holos up [-f holos.yaml]             start all services
holos down [-f holos.yaml]           stop and remove all services
holos ps                             list running projects
holos start [-f holos.yaml] [svc]    start a stopped service or all services
holos stop [-f holos.yaml] [svc]     stop a service or all services
holos console [-f holos.yaml] <inst> attach serial console to an instance
holos exec [-f holos.yaml] <inst> [cmd...]
                                     ssh into an instance (project's generated key)
holos logs [-f holos.yaml] <svc>     show service logs
holos validate [-f holos.yaml]       validate compose file
holos pull <image>                   pull a cloud image (e.g. alpine, ubuntu:noble)
holos images                         list available images
holos devices [--gpu]                list PCI devices and IOMMU groups
holos install [-f holos.yaml] [--system] [--enable]
                                     emit a systemd unit so the project survives reboot
holos uninstall [-f holos.yaml] [--system]
                                     remove the systemd unit written by `holos install`
holos import [vm...] [--all] [--xml file] [--connect uri] [-o file]
                                     convert virsh-defined VMs into a holos.yaml
```

## Compose File

The `holos.yaml` format is deliberately similar to docker-compose:

- **services** - each service is a VM with its own image, resources, and cloud-init config
- **depends_on** - services start in dependency order
- **ports** - `"host:guest"` syntax, auto-incremented across replicas
- **volumes** - `"./source:/target:ro"` for bind mounts, `"name:/target"` for top-level named volumes
- **replicas** - run N instances of a service
- **cloud_init** - packages, write_files, runcmd -- standard cloud-init
- **stop_grace_period** - how long to wait for ACPI shutdown before SIGTERM/SIGKILL (e.g. `"30s"`, `"2m"`); defaults to 30s
- **healthcheck** - `test`, `interval`, `retries`, `start_period`, `timeout` to gate dependents
- top-level **volumes** block - declare named data volumes that persist across `holos down`

### Graceful shutdown

`holos stop` and `holos down` send QMP `system_powerdown` to the guest
(equivalent to pressing the power button), then wait up to
`stop_grace_period` for QEMU to exit on its own. If the guest doesn't
halt in time — or QMP is unreachable — the runtime falls back to SIGTERM,
then SIGKILL, matching docker-compose semantics.

```yaml
services:
  db:
    image: ubuntu:noble
    stop_grace_period: 60s    # flush DB buffers before hard stop
```

### Data volumes

Top-level `volumes:` declares named data stores that live under
`state_dir/volumes/<project>/<name>.qcow2` and are symlinked into each
instance's work directory. They survive `holos down` — tearing down a
project only removes the symlink, never the backing file.

```yaml
name: demo
services:
  db:
    image: ubuntu:noble
    volumes:
      - pgdata:/var/lib/postgresql

volumes:
  pgdata:
    size: 20G
```

Volumes attach as virtio-blk devices with a stable `serial=vol-<name>`,
so inside the guest they appear as `/dev/disk/by-id/virtio-vol-pgdata`.
Cloud-init runs an idempotent `mkfs.ext4` + `/etc/fstab` snippet on
first boot so there's nothing to configure by hand.

### Healthchecks and `depends_on`

A service with a healthcheck blocks its dependents from starting until
the check passes. The probe runs via SSH (same key `holos exec` uses):

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
    depends_on: [db]     # waits for db to be healthy
```

`test:` accepts either a list (exec form) or a string (wrapped in
`sh -c`). Set `HOLOS_HEALTH_BYPASS=1` to skip the actual probe — handy
for CI environments without in-guest SSHD.

### holos exec

Every `holos up` auto-generates a per-project SSH keypair under
`state_dir/ssh/<project>/` and injects the public key via cloud-init.
A host port is allocated for each instance and forwarded to guest port
22, so you can:

```bash
holos exec web-0                 # interactive shell
holos exec db-0 -- pg_isready    # one-off command
```

`-u <user>` overrides the login user (defaults to the service's
`cloud_init.user`, or `ubuntu`).

### Reboot survival

Emit a systemd unit so a project comes back up after the host reboots:

```bash
holos install --enable           # per-user, no sudo needed
holos install --system --enable  # host-wide, before any login
holos install --dry-run          # print the unit and exit
```

User units land under `~/.config/systemd/user/holos-<project>.service`;
system units under `/etc/systemd/system/`. `holos uninstall` reverses
it (and is idempotent — safe to call twice).

### Networking

Every service can reach every other service by name. Under the hood:

- Each VM gets two NICs: user-mode (for host port forwarding) and socket multicast (for inter-VM L2)
- Static IPs are assigned automatically on the internal `10.10.0.0/24` segment
- `/etc/hosts` is populated via cloud-init so `db`, `web-0`, `web-1` all resolve
- No libvirt. No bridge configuration. No root required for inter-VM networking.

### GPU Passthrough

Pass physical GPUs (or any PCI device) directly to a VM via VFIO:

```yaml
services:
  ml:
    image: ubuntu:noble
    vm:
      vcpu: 8
      memory_mb: 16384
    devices:
      - pci: "01:00.0"       # GPU
      - pci: "01:00.1"       # GPU audio
    ports:
      - "8888:8888"
```

What holos handles:

- UEFI boot is enabled automatically when devices are present (OVMF firmware)
- `kernel-irqchip=on` is set on the machine for NVIDIA compatibility
- Per-instance OVMF_VARS copy so each VM has its own EFI variable store
- Optional `rom_file` for custom VBIOS ROMs

What you handle (host setup):

- Enable IOMMU in BIOS and kernel (`intel_iommu=on` or `amd_iommu=on`)
- Bind the GPU to `vfio-pci` driver
- Run `holos devices --gpu` to find PCI addresses and IOMMU groups

### Images

Use pre-built cloud images instead of building your own:

```yaml
services:
  web:
    image: alpine           # auto-pulled and cached
  api:
    image: ubuntu:noble     # specific tag
  db:
    image: debian           # defaults to debian:12
```

Available: `alpine`, `arch`, `debian`, `ubuntu`, `fedora`. Run `holos images` to see all tags.

### Dockerfile

Use a Dockerfile to provision a VM. `RUN`, `COPY`, `ENV`, and `WORKDIR` instructions are converted into a shell script that runs via cloud-init:

```yaml
services:
  api:
    dockerfile: ./Dockerfile
    ports:
      - "3000:3000"
```

```dockerfile
FROM ubuntu:noble

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y nodejs npm
COPY server.js /opt/app/
WORKDIR /opt/app
RUN npm init -y && npm install express
```

When `image` is omitted, the base image is taken from the Dockerfile's `FROM` line. The Dockerfile's instructions run before any `cloud_init.runcmd` entries.

Supported: `FROM`, `RUN`, `COPY`, `ENV`, `WORKDIR`. Unsupported instructions (`CMD`, `ENTRYPOINT`, `EXPOSE`, etc.) are silently skipped. `COPY` sources are resolved relative to the Dockerfile's directory and must be files, not directories — use `volumes` for directory mounts.

### Extra QEMU Arguments

Pass arbitrary flags straight to `qemu-system-x86_64` with `extra_args`:

```yaml
services:
  gpu:
    image: ubuntu:noble
    vm:
      vcpu: 4
      memory_mb: 8192
      extra_args:
        - "-device"
        - "virtio-gpu-pci"
        - "-display"
        - "egl-headless"
```

Arguments are appended after all holos-managed flags. No validation -- you own it.

### Resource Defaults

| Field | Default |
|-------|---------|
| replicas | 1 |
| vm.vcpu | 1 |
| vm.memory_mb | 512 |
| vm.machine | q35 |
| vm.cpu_model | host |
| cloud_init.user | ubuntu |
| image_format | inferred from extension |

### Import from virsh

Already running VMs under libvirt? `holos import` reads libvirt domain
XML and emits an equivalent `holos.yaml` so you can move existing
workloads onto holos without retyping every field.

```bash
holos import web-prod db-prod                # via `virsh dumpxml`
holos import --all -o holos.yaml             # every defined domain
holos import --xml ./web.xml                 # offline, no virsh needed
holos import --connect qemu:///system api    # non-default libvirt URI
```

The mapping covers the fields holos has a direct equivalent for:

| libvirt                            | holos                        |
|------------------------------------|------------------------------|
| `<vcpu>`                           | `vm.vcpu`                    |
| `<memory>` / `<currentMemory>`     | `vm.memory_mb`               |
| `<os><type machine="pc-q35-…">`    | `vm.machine` (collapsed)     |
| `<cpu mode="host-passthrough">`    | `vm.cpu_model: host`         |
| `<os><loader>`                     | `vm.uefi: true`              |
| first `<disk type="file">`         | `image:` + `image_format:`   |
| `<hostdev type="pci">`             | `devices: [{pci: …}]`        |

Anything holos can't translate cleanly — extra disks, bridged NICs,
USB passthrough, custom emulators — is reported as a warning on stderr
so you know what to revisit before `holos up`. Output goes to stdout
unless you pass `-o`, so it composes with shell redirection
(`holos import vm > holos.yaml`).

## Build

```bash
go build -o bin/holos ./cmd/holos
```

Build a guest image (requires mkosi):

```bash
./scripts/build-image.sh
```

## Host Requirements

- `/dev/kvm`
- `qemu-system-x86_64`
- `qemu-img`
- One of `cloud-localds`, `genisoimage`, `mkisofs`, or `xorriso`
- `mkosi` (only for building the base image)

## Non-Goals

This is not Kubernetes. It does not try to solve:

- Multi-host clustering
- Live migration
- Service meshes
- Overlay networks
- Scheduler, CRDs, or control plane quorum

The goal is to make KVM workable for single-host stacks without importing the operational shape of Kubernetes.

## License

Licensed under the [Apache License, Version 2.0](./LICENSE). See
[`NOTICE`](./NOTICE) for attribution.
