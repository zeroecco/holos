// Package images manages a curated registry of cloud images and handles
// downloading them into a local cache.
//
// # Registry
//
// The [Registry] variable contains pre-configured entries for Alpine, Arch,
// Debian, Ubuntu, and Fedora cloud images. Each entry maps a short reference
// (e.g. "alpine", "ubuntu:noble") to a download URL and disk format.
//
// # Image resolution
//
// [Resolve] interprets an image reference:
//
//   - Local paths (absolute, ./, ../, or ending in .qcow2/.raw/.img) are
//     returned as-is.
//   - Short names are matched against the registry by name and optional tag.
//   - Unrecognized references produce an error suggesting "holos images".
//
// # Pulling
//
// [Pull] resolves a reference and, for registry images, downloads to the
// cache directory if not already present. Downloads use a .part temporary
// file and print a SHA-256 summary on completion. Cache filenames include
// a truncated hash of the source URL for uniqueness.
package images
