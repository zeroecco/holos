#!/usr/bin/env bash
set -euo pipefail

HOLOS_BIN=${HOLOS_BIN:-./bin/holos}
case "$HOLOS_BIN" in
  /*) ;;
  *) HOLOS_BIN="$(pwd)/$HOLOS_BIN" ;;
esac
STATE_DIR=${HOLOS_STATE_DIR:?HOLOS_STATE_DIR must be set}

# Cover every supported distro family plus the newest non-default Debian/Ubuntu
# tags that have historically exposed boot differences.
DEFAULT_IMAGES=(
  alpine
  arch
  debian:bookworm
  debian:trixie
  ubuntu:noble
  ubuntu:resolute
  fedora
  almalinux
  rocky
  centos-stream
)

if [[ -n "${HOLOS_KVM_IMAGES:-}" ]]; then
  # shellcheck disable=SC2206
  IMAGES=(${HOLOS_KVM_IMAGES})
else
  IMAGES=("${DEFAULT_IMAGES[@]}")
fi

project_name() {
  local image=$1
  local name
  name=$(printf '%s' "$image" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')
  printf 'kvm-%s' "$name"
}

console_smoke() {
  local project=$1

  "$PYTHON_BIN" - "$HOLOS_BIN" "$project" <<'PY'
import os
import pty
import select
import subprocess
import sys
import time

holos, project = sys.argv[1], sys.argv[2]
master, slave = pty.openpty()
proc = subprocess.Popen(
    [holos, "console", project],
    stdin=slave,
    stdout=slave,
    stderr=slave,
    close_fds=True,
)
os.close(slave)

deadline = time.time() + 20
buf = b""
try:
    while time.time() < deadline:
        ready, _, _ = select.select([master], [], [], 0.25)
        if ready:
            chunk = os.read(master, 4096)
            if not chunk:
                break
            buf += chunk
            if b"Connected to serial console" in buf:
                os.write(master, b"\x1d")
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    raise
                if proc.returncode != 0:
                    sys.stderr.write(buf.decode("utf-8", "replace"))
                    raise SystemExit(proc.returncode)
                raise SystemExit(0)
        if proc.poll() is not None:
            break
finally:
    try:
        os.close(master)
    except OSError:
        pass

if proc.poll() is None:
    proc.kill()
sys.stderr.write(buf.decode("utf-8", "replace"))
raise SystemExit(f"console attach did not reach connected state for {project}")
PY
}

stop_from_without_compose() {
  local project=$1
  local dir
  dir=$(mktemp -d)
  (
    cd "$dir"
    "$HOLOS_BIN" stop "$project"
  )
  rm -rf "$dir"
}

PYTHON_BIN=${PYTHON_BIN:-python3}

cleanup() {
  for image in "${IMAGES[@]}"; do
    "$HOLOS_BIN" down "$(project_name "$image")" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

echo "holos binary: $HOLOS_BIN"
echo "state dir:    $STATE_DIR"
echo "images:       ${IMAGES[*]}"

"$HOLOS_BIN" doctor

for image in "${IMAGES[@]}"; do
  project=$(project_name "$image")
  echo "::group::${image}"

  "$HOLOS_BIN" pull "$image"
  "$HOLOS_BIN" verify "$image"

  "$HOLOS_BIN" run --name "$project" "$image" -- true
  "$HOLOS_BIN" exec -w 10m "$project" -- true
  console_smoke "$project"
  stop_from_without_compose "$project"
  "$HOLOS_BIN" down "$project"

  echo "::endgroup::"
done
