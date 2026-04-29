# Security Policy

## Supported Versions

holos is pre-1.0. Security fixes are shipped in the latest tagged release only.
Please upgrade to the newest release before reporting an issue unless the bug is
present on `main`.

## Reporting a Vulnerability

Please report security issues privately by emailing `security@zeroecco.com`.
Include:

- A short description of the issue and impact.
- Steps to reproduce, including the `holos.yaml` or command line involved.
- Host OS, holos version, QEMU version, and whether `/dev/kvm` is available.
- Any logs that help explain the behavior. Redact secrets from cloud-init
  `write_files`, `runcmd`, SSH keys, and image URLs before sending.

You should receive an acknowledgement within 72 hours. I will share a fix plan,
expected release timing, and whether a CVE makes sense once the issue is
confirmed.

## Scope

In scope:

- Path traversal or injection through project names, systemd units, QEMU option
  strings, Dockerfile parsing, or compose file fields.
- Leaks of cloud-init seed material, generated SSH keys, or cached image
  credentials.
- Cache corruption or unsafe image promotion in `holos pull` / `holos up`.
- Escapes from the intended compose build context when translating Dockerfiles.

Out of scope:

- Vulnerabilities in guest operating systems or packages installed inside VMs.
- Misconfigured host kernel, KVM, VFIO, IOMMU, or QEMU installations.
- Issues requiring malicious root access on the host.
- Denial of service from intentionally huge images, volumes, or command output.

## Hardening Notes

holos keeps runtime state owner-only where it may contain secrets:
`state_dir`, instance workdirs, generated cloud-init seed files, and SSH keys are
created with restrictive permissions. Treat any compose file containing
`cloud_init.write_files`, `cloud_init.runcmd`, private image URLs, or custom SSH
keys as sensitive operational material.

## Threat Model And Hardening Guide

holos is a single-host VM launcher. It assumes the host user running holos is
trusted to start QEMU processes, read the project state directory, and access any
bind-mounted host paths. It does not try to sandbox a malicious local user who
already controls the holos state tree, the QEMU binary, or root on the host.

Primary boundaries:

- Guest isolation comes from KVM/QEMU and the guest kernel boundary. Keep the
  host kernel, QEMU, OVMF, and guest packages patched.
- Compose files are trusted configuration. Review `vm.extra_args`,
  `cloud_init.runcmd`, `cloud_init.write_files`, bind mounts, and private image
  URLs before running a project from another person.
- Built-in images are downloaded over HTTPS and verified against upstream
  checksum metadata before entering the cache. Re-run `holos verify --all` to
  audit cached images, and prefer pinned local images for highly controlled
  environments.
- Project lifecycle operations take a per-project lock so concurrent `up`,
  `down`, `start`, `stop`, and state refreshes cannot interleave their state
  writes for the same project.
- State, generated SSH keys, cloud-init seeds, volume qcow2 files, and run
  compose files are intended to be owner-only. Do not put `HOLOS_STATE_DIR` on a
  shared writable filesystem.
- Bind mounts deliberately expose host files to the guest. Use `:ro` where
  possible and mount only the minimum directory needed by the workload.
- Named volumes are persistent qcow2 disks. Treat them like guest disks: scan or
  inspect them before attaching data produced by an untrusted VM to another VM.
- Port forwards bind on `127.0.0.1`. Expose them beyond localhost only through a
  deliberate reverse proxy, firewall rule, or SSH tunnel.
- VFIO passthrough gives a guest direct device access. Use IOMMU isolation,
  understand the device reset behavior, and do not pass through devices that
  share an unsafe IOMMU group.

Operational hardening checklist:

- Run `holos doctor` before first use and after host upgrades.
- Keep holos updated to the latest release and verify release checksums before
  installing binaries.
- Run `holos verify --all` after pulling images and before important rebuilds.
- Use explicit `image_os` metadata for local/custom images so cloud-init does
  not infer behavior from a filename.
- Prefer `holos exec` over password login; holos does not add guest passwords.
- Review `console.log` after first boot, especially when using named volumes:
  mount failures are emitted there and fail the cloud-init command.
