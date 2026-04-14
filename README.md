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
holos stop [-f holos.yaml] [svc]     stop a service or all services
holos console [-f holos.yaml] <inst> attach serial console to an instance
holos logs [-f holos.yaml] <svc>     show service logs
holos validate [-f holos.yaml]       validate compose file
holos pull <image>                   pull a cloud image (e.g. alpine, ubuntu:noble)
holos images                         list available images
holos devices [--gpu]                list PCI devices and IOMMU groups
```

## Compose File

The `holos.yaml` format is deliberately similar to docker-compose:

- **services** - each service is a VM with its own image, resources, and cloud-init config
- **depends_on** - services start in dependency order
- **ports** - `"host:guest"` syntax, auto-incremented across replicas
- **volumes** - `"./source:/target:ro"` syntax, mounted via virtfs
- **replicas** - run N instances of a service
- **cloud_init** - packages, write_files, runcmd -- standard cloud-init

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
