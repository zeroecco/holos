// Package runtime manages the lifecycle of QEMU VM instances and persists
// project state to disk.
//
// # Manager
//
// [Manager] is the central coordinator. It provides:
//
//   - [Manager.Up]: bring a compose project to its desired state, starting
//     services in dependency order. If the spec hash has changed since the
//     last run, the entire project is torn down and recreated.
//   - [Manager.Down]: stop all VMs and remove all state for a project.
//   - [Manager.StopProject] / [Manager.StopService]: stop VMs without
//     removing disk state, allowing restart with preserved overlays.
//   - [Manager.ProjectStatus]: refresh PID liveness and return current state.
//   - [Manager.ListProjects]: enumerate all known projects.
//
// # State directory
//
// State is stored under a configurable directory (default ~/.local/state/holos
// for non-root, /var/lib/holos for root, or HOLOS_STATE_DIR):
//
//	<state>/projects/<name>.json   per-project JSON record
//	<state>/instances/<project>/<service>-<index>/
//	    root.qcow2        copy-on-write overlay
//	    seed.img|seed.iso cloud-init NoCloud media
//	    console.log       serial console log
//	    serial.sock       serial console Unix socket
//	    qmp.sock          QMP control socket
//	    qemu.log          QEMU stderr/stdout
//	    OVMF_VARS.fd      per-instance EFI variable store (UEFI only)
//
// # Instance lifecycle
//
// Fresh instances get a new qcow2 overlay, cloud-init seed, and allocated
// ports. Stopped instances with an existing work directory are restarted
// in-place, preserving disk state. Instances are started in a new session
// (setsid) and detached; a 300ms liveness check catches early QEMU failures.
//
// Stopping sends SIGTERM with a 10-second grace period, then SIGKILL.
//
// # Port allocation
//
// Explicit host ports are incremented by the replica index and verified
// available on 127.0.0.1. When the host port is 0, an ephemeral port is
// allocated by binding to :0.
//
// # UEFI
//
// OVMF firmware paths are discovered via HOLOS_OVMF_CODE / HOLOS_OVMF_VARS
// environment variables or a built-in search list. The OVMF_VARS template is
// copied per-instance so each VM has its own EFI variable store.
//
// # Cloud-init seed
//
// Seed media is created by the first available tool: cloud-localds (→ .img),
// genisoimage/mkisofs (→ .iso), or xorriso (→ .iso with cidata volume ID).
package runtime
