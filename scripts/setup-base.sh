#!/usr/bin/env bash

set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

apt-get update -qq
apt-get install -y --no-install-recommends \
    cloud-init \
    openssh-server \
    qemu-guest-agent

systemctl enable qemu-guest-agent.service
systemctl enable ssh.service
systemctl enable serial-getty@ttyS0.service
systemctl enable systemd-networkd.service
systemctl enable systemd-resolved.service

mkdir -p /etc/cloud/cloud.cfg.d
cat > /etc/cloud/cloud.cfg.d/99-holosteric.cfg <<'EOF'
datasource_list: [ NoCloud, None ]
manage_etc_hosts: true
EOF

mkdir -p /etc/systemd/system/getty.target.wants

apt-get clean
rm -rf /var/lib/apt/lists/*
