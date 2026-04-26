// Package dockerfile converts a subset of Dockerfile instructions into
// cloud-init artifacts suitable for provisioning a VM.
//
// [Parse] reads a Dockerfile and extracts:
//
//   - The base image reference from FROM (used when the compose service
//     omits an explicit image).
//   - A shell script assembled from RUN, ENV, and WORKDIR instructions.
//   - A set of [config.WriteFile] entries from COPY instructions (file
//     sources only; directories are not supported).
//
// The generated script is written into the VM at /var/lib/holos/build.sh
// and executed via cloud-init runcmd before any user-specified commands.
//
// Unsupported instructions (CMD, ENTRYPOINT, EXPOSE, LABEL, VOLUME,
// HEALTHCHECK, STOPSIGNAL, SHELL, ONBUILD, ARG, USER, ADD) are rejected with
// an error so Dockerfile behavior is never silently dropped.
// Multi-stage builds (COPY --from=) are also rejected.
package dockerfile
