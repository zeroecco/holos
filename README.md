<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./docs/holos-lockup-dark.svg">
  <img alt="holos" src="./docs/holos-lockup-light.svg" width="340">
</picture>

Docker compose for KVM. Define multi-VM stacks in one YAML file. No libvirt, no
XML, no distributed control plane.

Website and docs: <https://zeroecco.github.io/holos/>

The primitive is a VM, not a container. Every workload instance gets its own
kernel boundary, qcow2 overlay, cloud-init seed, and generated SSH access.

## Start here

Run VMs on a Linux host with `/dev/kvm`, QEMU, and a cloud-init seed builder.
See [host requirements](#host-requirements). macOS builds support authoring,
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

Or, from a checkout of this repository with Go 1.26 installed, build from source:

```bash
go build -o bin/holos ./cmd/holos
export PATH="$PWD/bin:$PATH"
holos doctor
```

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

## Daily use

Inside a project directory:

```bash
holos exec web       # guest shell; exit returns to the host
holos logs web      # boot and provisioning logs
holos stop web      # keep its disk
holos start web
holos up --dry-run  # preview changes after editing holos.yaml
holos help exec     # syntax and options
```

Use an instance name such as `web-0` when a service has multiple replicas.
Changes that replace or remove a VM require `holos up --recreate`; check the
preview first because replacement deletes that VM's root disk. Named volumes
survive. See the [update guide](./docs/cli.md#previewing-and-applying-configuration-changes)
before updating projects created by an older release.

When finished with this disposable stack:

```bash
holos down hello
```

`stop` preserves instance disks. `down` deletes them; named volumes remain until
explicitly removed. See the [CLI guide](./docs/cli.md) for snapshots, volumes,
image locks, and systemd setup.

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
- [Missing features](./docs/missing-features.md): prioritized gaps that fit
  holos' single-host KVM compose goal.
- [Development](./docs/development.md): build, test, host requirements, and
  release process.
- [Security policy](./SECURITY.md): supported versions and private reporting.
- [Threat model / hardening](./docs/threat-model.md):
  image verification, state permissions, locks, and operational guidance.
- [Contributing](./CONTRIBUTING.md): build, test, style, and PR conventions.

## Host Requirements

- Linux with `/dev/kvm`
- `qemu-system-x86_64`
- `qemu-img`
- One of `cloud-localds`, `genisoimage`, `mkisofs`, or `xorriso`
- OVMF / edk2-ovmf firmware for UEFI or PCI passthrough
- `ssh` for `holos exec` and healthchecks

Run `holos doctor` to check the host.

## Troubleshooting

### Permission denied opening /dev/kvm

Check `id -nG` and `ls -l /dev/kvm`. On hosts where the device belongs to the
`kvm` group, add your user with `sudo usermod -aG kvm "$USER"`, then log out
and reconnect. Existing shells keep their old group membership. Run
`holos doctor` again from the new session.

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
