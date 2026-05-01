---
title: Threat Model
description: Security boundaries, image verification, locking, and operational hardening for holos.
permalink: /threat-model/
---

# Threat Model

holos is a single-host VM launcher. It assumes the host user running holos is
trusted to start QEMU processes, read the project state directory, and access any
bind-mounted host paths. It does not try to sandbox a malicious local user who
already controls the holos state tree, the QEMU binary, or root on the host.

## Primary Boundaries

- Guest isolation comes from KVM/QEMU and the guest kernel boundary. Keep the
  host kernel, QEMU, OVMF, and guest packages patched.
- Compose files are trusted configuration. Review `vm.extra_args`,
  `cloud_init.runcmd`, `cloud_init.write_files`, bind mounts, PCI passthrough,
  and private image URLs before running a project from another person.
- Built-in images are downloaded over HTTPS and verified against upstream
  checksum metadata before entering the cache. Re-run `holos verify --all` to
  audit cached images before important rebuilds.
- Project lifecycle operations take a per-project lock so concurrent `up`,
  `down`, `start`, `stop`, and state refreshes cannot interleave state writes
  for the same project. Commands wait for the lock up to `--lock-timeout`
  (default `5m`), and `--no-wait` fails immediately with lock diagnostics.
- State, generated SSH keys, cloud-init seeds, run compose files, and volume
  qcow2 files are intended to be owner-only. Do not put `HOLOS_STATE_DIR` on a
  shared writable filesystem.
- Bind mounts deliberately expose host files to the guest. Use `:ro` where
  possible and mount only the minimum directory needed by the workload.
- Named volumes are persistent qcow2 disks. Treat them like guest disks: scan or
  inspect data produced by an untrusted VM before attaching it to another VM.
- Port forwards bind on `127.0.0.1`. Expose them beyond localhost only through a
  deliberate reverse proxy, firewall rule, or SSH tunnel.
- VFIO passthrough gives a guest direct device access. Use IOMMU isolation,
  understand device reset behavior, and do not pass through devices that share
  an unsafe IOMMU group.

## Custom And Private Images

Local image paths (`./image.qcow2`, `/srv/images/base.raw`, etc.) are treated as
operator-supplied artifacts. holos can infer the disk format from the extension,
but it cannot know whether a private qcow2 is authentic, patched, or built with
the expected cloud-init user and init system.

For teams using private images:

- Store images in a controlled artifact registry or object store, not as
  mutable files in a shared home directory.
- Publish a checksum beside every image and verify it before running `holos up`.
  For example:

  ```bash
  sha256sum -c images/SHA256SUMS
  holos up
  ```

- Prefer immutable image filenames that include a build id, date, or digest
  prefix, such as `api-base-2026-05-01-3f2a9c.qcow2`.
- Set `image_format` and `image_os` explicitly in `holos.yaml` so private image
  behavior does not depend on filename conventions.
- Keep private image URLs and compose files with credentials out of logs and
  issue trackers.

## Project Image Lockfiles

For reproducible environments, keep a project-owned image lockfile in source
control next to `holos.yaml`. holos does not yet enforce a first-class lockfile,
but a simple `holos.images.lock` or `images/SHA256SUMS` gives reviewers and CI a
stable contract:

```text
sha256  ./images/api-base-2026-05-01.qcow2  3f2a...
sha256  ./images/db-base-2026-05-01.qcow2   a91c...
```

CI can verify those hashes before `holos validate` or before deploying to a KVM
host. The same file also documents which private image build a compose change
was tested against.

## Hardening Checklist

- Run `holos doctor` before first use and after host upgrades.
- Keep holos updated to the latest release and verify release checksums and
  GitHub artifact attestations before installing binaries.
- Run `holos verify --all` after pulling built-in images and before important
  rebuilds.
- Verify private/local image checksums with a project lockfile or checksum
  manifest before launching them.
- Use explicit `image_os` metadata for local/custom images so cloud-init does
  not infer behavior from a filename.
- Prefer `holos exec` over password login; holos does not add guest passwords.
- Review `console.log` after first boot, especially when using named volumes:
  mount failures are emitted there and fail the cloud-init command.
