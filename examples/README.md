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

## `examples/simple-inference`

A GPU-passthrough DeepSeek inference template in an Ubuntu VM. It installs
Ollama, pulls `deepseek-r1:7b`, publishes Ollama's API on host port 11434, and
stores model files on a named volume. Replace the PCI addresses and install the
guest GPU driver for your card before running it.

```bash
holos devices --gpu
holos up -f examples/simple-inference/holos.yaml
curl http://localhost:11434/api/generate \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-r1:7b","prompt":"Explain KVM in one sentence.","stream":false}'
holos down simple-inference
```

Use this when you want a useful local model-serving VM backed by a physical GPU.

## `examples/minecraft-server`

A vanilla Minecraft Java server in an Ubuntu VM. It downloads the latest
official server jar on first boot, runs it under systemd, publishes port 25565,
and stores the world on a named volume.

```bash
holos up -f examples/minecraft-server/holos.yaml
holos logs minecraft-server
holos down minecraft-server
```

Use this when you want a stateful game-server example with a healthcheck.

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
