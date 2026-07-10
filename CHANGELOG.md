# Changelog

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
