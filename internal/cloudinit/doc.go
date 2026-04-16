// Package cloudinit renders NoCloud seed data from a service manifest.
//
// [Render] produces three strings for a given instance:
//
//   - user-data: a #cloud-config YAML document that provisions the VM user,
//     installs packages, writes files, and runs commands.
//   - meta-data: instance-id and local-hostname.
//   - network-config: a netplan v2 document (empty when there is no internal
//     network) that assigns a static IP on the internal NIC and DHCP on the
//     external NIC.
//
// Every instance automatically gets serial console access: a GRUB config for
// console=ttyS0 and a systemd serial-getty autologin override for the
// configured cloud-init user.
//
// When the manifest carries ExtraHosts (populated by the compose resolver for
// inter-VM name resolution), a custom /etc/hosts is written and
// manage_etc_hosts is disabled.
package cloudinit
