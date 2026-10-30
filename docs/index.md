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
    <pre><code>$ holos run --name try-holos alpine
$ holos exec -w 5m try-holos
# In the guest: explore, then exit.
$ holos stop try-holos
$ holos start try-holos
$ holos down try-holos</code></pre>
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

Run VMs on a Linux host with `/dev/kvm`, QEMU, and a cloud-init seed builder.
See [host requirements](./development.md#host-requirements). macOS builds support authoring,
image downloads, and validation; they cannot run KVM VMs.

## Install

Pre-built binaries are attached to every
[GitHub release](https://github.com/zeroecco/holos/releases):

This example installs the Linux x86-64 build. It uses `curl`, `tar`,
`sha256sum`, and the GitHub CLI (`gh`) for provenance verification. For other
platforms, choose the matching archive on the release page.

```bash
(
set -e
TAG=v0.7.1
ASSET=holos_${TAG#v}_Linux_x86_64.tar.gz
BASE=https://github.com/zeroecco/holos/releases/download/$TAG
curl -fLO $BASE/$ASSET
curl -fLO $BASE/checksums.txt
grep " $ASSET$" checksums.txt | sha256sum -c -
gh attestation verify $ASSET --repo zeroecco/holos
sudo tar -xz -C /usr/local/bin -f $ASSET holos
holos version
holos doctor
)
```

Release pages include SHA-256 checksums and GitHub artifact attestations for
signed provenance. Verify both before installing binaries on production hosts.

## Try one VM

Use a fixed name so every command below works as written:

```bash
holos run --name try-holos alpine
holos exec -w 5m try-holos
```

You are now in the guest. Run `cat /etc/os-release`, then `exit` to return to
the host. From the host:

```bash
holos stop try-holos    # keep the guest disk
holos start try-holos
holos exec try-holos
```

Exit the guest again, then remove the disposable VM:

```bash
holos down try-holos   # deletes its guest disk
```

## Run a web service

Create an empty directory and save this as `holos.yaml`:

```yaml
name: hello

services:
  web:
    image: ubuntu:noble
    ports:
      - "8080:80"
    cloud_init:
      packages: [nginx]
      write_files:
        - path: /var/www/html/index.html
          content: "hello from holos\n"
      runcmd:
        - systemctl restart nginx
```

From that directory:

```bash
holos up
holos exec -w 5m web -- sudo cloud-init status --wait
curl --fail localhost:8080
```

Expect `hello from holos`. The wait matters: `up` starts the VM, but nginx is
installed during first boot. If provisioning fails, read `holos logs web`.

When finished, run `holos down hello` to delete this stack's instance disks.
Use `holos stop web` instead when you want to keep the disk and resume later.

Next: [daily commands and safe updates](./cli.md) or
[the Compose file reference](./compose.md).

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
  <div class="card">
    <h3><a href="{{ '/missing-features/' | relative_url }}">Missing Features</a></h3>
    <p>Prioritized gaps that still fit holos' single-host KVM compose goal.</p>
  </div>
</div>
