# Changelog

## Unreleased

### Security

- Upgrade `golang.org/x/crypto` to 0.52.0 to fix five reachable SSH
  vulnerabilities affecting console, exec, and SSH health-check paths.
- Add a pinned `govulncheck` CI gate and weekly dependency update checks for Go
  modules and GitHub Actions.
- Validate GitHub Actions workflow syntax in CI and bound every CI job with an
  explicit timeout.
- Pin every third-party workflow action to an immutable commit and limit
  release/Pages write permissions to their publishing jobs; release builds now
  use an exact GoReleaser version instead of a moving major-version range.
- Tighten image cache directories to owner-only permissions, including caches
  created by older releases.

### Reliability

- Persist project records with a synced temporary file and atomic rename so a
  crash or full filesystem cannot truncate the only lifecycle state record.
- Validate project identities again at the persistence boundary to prevent an
  invalid in-memory record from escaping the state directory.

## [0.6.1] - 2026-07-10

This patch release completes the operational safeguards introduced in 0.6.0.

### Added

- Restore stopped root-overlay and detached-volume qcow2 snapshots.
- Export snapshots as standalone qcow2 images for copying or reuse on another
  host.
- Cgroup-aware capacity checks for CPU and memory limits.
- Stronger network preflight checks for multicast ports, bridge helper access,
  tap prerequisites, and `/dev/net/tun` access.
- `holos up --lockfile <path>` for projects whose image lockfile is not next to
  the compose file.

### Tests and documentation

- Added focused regression coverage for snapshot restore/export, image-lock
  modes, cgroup capacity, and network validation.
- Updated CLI, Compose, and threat-model documentation to match the current
  lockfile and snapshot behavior.

[0.6.1]: https://github.com/zeroecco/holos/releases/tag/v0.6.1

## [0.6.0] - 2026-07-09

This release improves day-to-day operation of single-host VM stacks without
adding a background control plane.

### Added

- Snapshot inventory and deletion for stopped root overlays:
  `holos snapshots list` and `holos snapshots rm`.
- Snapshot inventory and deletion for detached named volumes:
  `holos volumes snapshots` and `holos volumes snapshot-rm`.
- Strict image reproducibility with `holos up --locked`, which requires the
  adjacent `holos.images.lock` file and rejects image drift.
- Host preflight checks:
  - `holos validate --capacity` checks aggregate replica CPU and memory.
  - `holos validate --network` checks required bridge/tap interfaces.
- Built-in Bash, Zsh, and Fish completion scripts via
  `holos completion <bash|zsh|fish>`.

### Documentation

- Expanded CLI and Compose documentation for snapshot lifecycle operations,
  image locks, capacity checks, network checks, and shell completion.
- Clarified that service naming is deterministic generated `/etc/hosts`
  configuration; holos does not run a DNS or service-supervision daemon.

### Upgrade notes

- `holos up --locked` reads `holos.images.lock` next to the compose file. Use
  `holos images lock -f holos.yaml` to create or refresh it.
- Snapshot operations still require the target root overlay or volume to be
  detached/stopped. They are qcow2 internal snapshots, not portable exports.
- `holos validate --capacity` and `--network` are opt-in checks; normal
  validation behavior is unchanged.

[0.6.0]: https://github.com/zeroecco/holos/releases/tag/v0.6.0
