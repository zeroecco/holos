// Package compose loads and resolves holos.yaml compose files.
//
// A compose file declares a multi-VM stack in a docker-compose-like YAML
// format. [Load] parses the YAML into a [File], then [File.Resolve] validates
// names and dependency order, plans the internal network, pulls or locates
// cloud images, merges any Dockerfile-derived provisioning, and produces a
// fully resolved [Project] ready for the runtime to execute.
//
// # Compose file lookup
//
// [FindFile] searches a directory for holos.yaml (or holos.yml).
// If the file omits a project name, the parent directory basename is used.
//
// # Service resolution
//
// Each service is validated (DNS-safe names, acyclic depends_on) and converted
// to a [config.Manifest]. The resolver:
//
//   - Topologically sorts services so dependencies start first.
//   - Assigns static IPs on the 10.10.0.0/24 internal subnet.
//   - Generates deterministic MAC addresses from the project and service names.
//   - Auto-enables UEFI when PCI devices are attached.
//   - Merges Dockerfile instructions (RUN, COPY, ENV, WORKDIR) into cloud-init.
//
// # Spec hashing
//
// The resolved [Project] carries a SpecHash (truncated SHA-256 of the
// JSON-marshaled [File]). The runtime uses this to detect config drift and
// trigger a full teardown-and-recreate when the compose file changes.
package compose
