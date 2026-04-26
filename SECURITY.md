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
