---
title: Docker Compose For KVM
description: Define multi-VM KVM stacks in one YAML file.
---

<section class="hero">
  <div>
    <p class="eyebrow">Docker compose for KVM</p>
    <h1>Real VMs, compose-shaped workflows.</h1>
    <p class="lead">holos launches cloud-image VMs with generated disks, cloud-init, SSH access, port forwards, healthchecks, volumes, and PCI passthrough from one readable YAML file.</p>
    <div class="actions">
      <a class="button" href="#quick-start">Quick Start</a>
      <a class="button secondary" href="{{ '/compose/' | relative_url }}">Read The Docs</a>
      <a class="button secondary" href="https://github.com/zeroecco/holos/releases">Install</a>
    </div>
  </div>
  <div class="terminal" aria-label="holos command example">
    <div class="terminal-bar"><span></span><span></span><span></span></div>
    <pre><code>$ holos run alpine
$ holos exec alpine
$ holos down alpine

$ holos up
$ curl localhost:8080
hello from holos</code></pre>
  </div>
</section>

## Why holos

<div class="cards">
  <div class="card">
    <h3>VMs As The Primitive</h3>
    <p>Every workload gets its own kernel boundary, qcow2 overlay, cloud-init seed, and generated SSH access.</p>
  </div>
  <div class="card">
    <h3>No Control Plane</h3>
    <p>Run multi-VM stacks on one Linux KVM host without libvirt XML, clusters, schedulers, or service meshes.</p>
  </div>
  <div class="card">
    <h3>Hardware Friendly</h3>
    <p>Use UEFI, volumes, healthchecks, Dockerfile-shaped provisioning, and VFIO PCI passthrough when the stack needs them.</p>
  </div>
</div>

## Quick Start

Runtime commands require Linux with `/dev/kvm`. macOS binaries are still useful for authoring and offline commands such as `validate`, `images`, `pull`, and `import`.

```bash
TAG=v0.2.3
ASSET=holos_${TAG#v}_Linux_x86_64.tar.gz
BASE=https://github.com/zeroecco/holos/releases/download/$TAG
curl -LO $BASE/$ASSET
curl -LO $BASE/checksums.txt
grep " $ASSET$" checksums.txt | sha256sum -c -
gh attestation verify $ASSET --repo zeroecco/holos
sudo tar -xz -C /usr/local/bin -f $ASSET holos
holos doctor
```

Save this as `holos.yaml`:

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

Then launch it:

```bash
holos up
curl localhost:8080
holos down hello
```

## Documentation

<div class="cards">
  <div class="card">
    <h3><a href="{{ '/compose/' | relative_url }}">Compose File</a></h3>
    <p>Services, networking, volumes, healthchecks, Dockerfile provisioning, PCI passthrough, and defaults.</p>
  </div>
  <div class="card">
    <h3><a href="{{ '/cli/' | relative_url }}">CLI Guide</a></h3>
    <p>Ad hoc VMs, SSH with <code>holos exec</code>, reboot survival, virsh import, and host diagnostics.</p>
  </div>
  <div class="card">
    <h3><a href="{{ '/examples/' | relative_url }}">Examples</a></h3>
    <p>Small runnable stacks and templates for nginx, Dockerfile provisioning, GPU passthrough, and multi-service demos.</p>
  </div>
  <div class="card">
    <h3><a href="{{ '/threat-model/' | relative_url }}">Threat Model</a></h3>
    <p>Security boundaries, lock behavior, image verification, private qcow2 guidance, and hardening checklists.</p>
  </div>
</div>
