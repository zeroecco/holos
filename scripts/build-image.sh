#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(dirname "$script_dir")"
config_file="$project_root/images/base/mkosi.conf"
output_dir="$project_root/output/images/base"
raw_image="$output_dir/image.raw"
qcow2_image="$output_dir/holosteric-base.qcow2"

for dependency in mkosi qemu-img; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        echo "missing dependency: $dependency" >&2
        exit 1
    fi
done

mkdir -p "$output_dir"

sudo mkosi --include "$config_file" build -O "$output_dir" --force
sudo qemu-img convert -f raw -O qcow2 "$raw_image" "$qcow2_image"

cat <<EOF
built image:
  raw:   $raw_image
  qcow2: $qcow2_image
EOF
