// Package config defines the canonical service manifest schema used between
// the compose layer and the runtime.
//
// A [Manifest] is the fully resolved description of a single service: image
// path, VM sizing, network mode, port forwards, filesystem mounts, cloud-init
// configuration, PCI device passthrough, and internal network parameters.
//
// The compose package builds Manifests during resolution; the runtime and qemu
// packages consume them to launch VM instances.
//
// # Defaults
//
// Zero-valued fields are populated by [LoadManifest] (or by the compose
// resolver) using the Default* constants: 1 vCPU, 512 MB RAM, q35 machine
// type, host CPU model, ubuntu user, and TCP protocol.
//
// # Validation
//
// [Manifest.Validate] enforces: DNS-safe names, replicas >= 1, image required,
// image format qcow2 or raw, memory >= 128 MB, network mode "user", ports
// 1-65535 TCP only, and non-empty mount/write_file paths.
package config
