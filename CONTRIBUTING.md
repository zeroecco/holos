# Contributing to holos

Thanks for taking a look. holos is a small Go codebase with no
runtime control plane and a hard policy against scope creep. See
[Non-Goals](./README.md#non-goals) before proposing large features.

## Development setup

Requires Go 1.26+ and (for end-to-end testing) a Linux host with
`/dev/kvm`, `qemu-system-x86_64`, `qemu-img`, and one of
`cloud-localds`, `genisoimage`, `mkisofs`, or `xorriso`.

```bash
git clone https://github.com/zeroecco/holos
cd holos
go build -o bin/holos ./cmd/holos
```

## Tests, vet, format

The full local check before opening a PR:

```bash
gofmt -l . cmd internal     # must print nothing
go vet ./...
go test ./... -timeout 60s
```

The `cmd/holos` and `internal/{compose,images,virtimport}` packages
all carry hermetic unit tests; no `/dev/kvm` required. End-to-end
integration mocks live under `test/integration/mocks/`.

## Code style

- `gofmt` is law. CI rejects unformatted code.
- Avoid narrating comments (`// open the file`, `// return the result`).
  Comments should explain *why*: non-obvious intent, trade-offs, or
  constraints the code can't convey on its own. Existing files use
  this style; please match.
- Prefer pure helpers that can be unit-tested over inline logic in
  command handlers. Recent examples: `resolveLogTargets`, `sshdReady`,
  `randHexFallback`.
- New behavior needs a test. New flags need a README mention.

## Commits and PRs

- One logical change per commit; squash WIP locally.
- Commit subjects in imperative mood: "fix exec retry race", not
  "fixed" or "fixes".
- PR descriptions should cover *why* the change exists, not just
  *what* it does. The diff already shows the what.
- Reference the issue you're fixing if there is one (`Fixes #N`).

## Reporting bugs

Open a [GitHub issue](https://github.com/zeroecco/holos/issues) with:

- `holos version` output
- Host kernel (`uname -a`) and qemu version (`qemu-system-x86_64 --version`)
- The minimal `holos.yaml` (or `holos run` invocation) that reproduces
- What you expected vs. what you saw

Security issues: see [SECURITY.md](./SECURITY.md) if present, otherwise
email the maintainer rather than opening a public issue.
