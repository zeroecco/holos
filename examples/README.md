# Examples

Each example is a complete `holos.yaml` you can copy or run with `-f`.
They are intentionally small and show one idea at a time.

## `examples/alpine-nginx`

A tiny Alpine VM that installs nginx, writes an index page with cloud-init, and
publishes guest port 80 on host port 8080.

```bash
holos up -f examples/alpine-nginx/holos.yaml
curl localhost:8080
holos down alpine-nginx
```

Use this when you want the smallest possible working stack.

## `examples/netboot-xyz`

A Dockerfile-backed VM that serves a static page. It demonstrates how holos
translates supported Dockerfile instructions (`FROM`, `RUN`, `COPY`, `ENV`,
`WORKDIR`) into first-boot provisioning.

```bash
holos up -f examples/netboot-xyz/holos.yaml
curl localhost:8080
holos down netboot-xyz
```

Use this when you want Dockerfile-shaped provisioning without building a
container image.

## `examples/gpu-passthrough`

A template for passing PCI devices through with VFIO. You must edit the PCI
addresses to match your host and complete IOMMU / `vfio-pci` setup first.

```bash
holos devices --gpu
holos validate -f examples/gpu-passthrough/holos.yaml
```

Use this as a checklist, not as a copy-paste runnable demo.

## `examples/holos.yaml`

A three-service stack (`db`, `api`, `web`) showing `depends_on`, cloud-init
packages, generated config files, and replicas. The image paths point at a
locally built base image, so adjust them before running.
