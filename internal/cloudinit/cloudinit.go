package cloudinit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

// Cloud-init user-data schema.

type cloudConfig struct {
	Hostname       string   `yaml:"hostname"`
	ManageEtcHosts bool     `yaml:"manage_etc_hosts"`
	SSHPwAuth      bool     `yaml:"ssh_pwauth"`
	PackageUpdate  bool     `yaml:"package_update,omitempty"`
	Packages       []string `yaml:"packages,omitempty"`
	Users          []ccUser `yaml:"users"`
	WriteFiles     []ccFile `yaml:"write_files,omitempty"`
	BootCmd        []string `yaml:"bootcmd,omitempty"`
	RunCmd         []string `yaml:"runcmd,omitempty"`
}

type ccUser struct {
	Name              string   `yaml:"name"`
	Groups            []string `yaml:"groups,omitempty"`
	Shell             string   `yaml:"shell,omitempty"`
	Sudo              string   `yaml:"sudo,omitempty"`
	SSHAuthorizedKeys []string `yaml:"ssh_authorized_keys,omitempty"`
}

type ccFile struct {
	Path        string `yaml:"path"`
	Content     string `yaml:"content"`
	Permissions string `yaml:"permissions"`
	Owner       string `yaml:"owner"`
}

// Cloud-init network-config schema (netplan v2).

type netConfig struct {
	Network netConfigBody `yaml:"network"`
}

type netConfigBody struct {
	Version   int                    `yaml:"version"`
	Ethernets map[string]ethernetDef `yaml:"ethernets"`
}

type ethernetDef struct {
	Match     matchDef `yaml:"match"`
	DHCP4     bool     `yaml:"dhcp4"`
	Addresses []string `yaml:"addresses,omitempty"`
}

type matchDef struct {
	MACAddress string `yaml:"macaddress"`
}

// osFamily enumerates the init-system / userland conventions we need to
// branch on when rendering cloud-init user-data.
type osFamily int

const (
	familySystemd osFamily = iota
	familyOpenRC
)

// serialGettySystemdCmd enables auto-login on ttyS0 via systemd. The whole
// chain is guarded by `command -v systemctl` so it is a no-op on non-systemd
// distros (e.g. Alpine/OpenRC).
const serialGettySystemdCmd = "command -v systemctl >/dev/null 2>&1 && { systemctl daemon-reload; systemctl enable serial-getty@ttyS0.service; systemctl restart serial-getty@ttyS0.service; } ; command -v update-grub >/dev/null 2>&1 && update-grub || true"

// Render produces cloud-init user-data, meta-data, and network-config.
// networkConfig is empty when there is no internal network.
func Render(manifest config.Manifest, instanceName string, instanceIndex int) (userData, metaData, networkConfig string) {
	family := osFamilyFromMetadata(manifest.ImageOS)

	cc := cloudConfig{
		Hostname:       hostname(manifest, instanceName),
		ManageEtcHosts: len(manifest.ExtraHosts) == 0,
		Users:          []ccUser{renderUser(manifest, family)},
	}

	if len(manifest.CloudInit.Packages) > 0 {
		cc.PackageUpdate = true
		cc.Packages = manifest.CloudInit.Packages
	}

	// System-managed write_files.
	if len(manifest.ExtraHosts) > 0 {
		cc.WriteFiles = append(cc.WriteFiles, ccFile{
			Path:        "/etc/hosts",
			Content:     hostsFileContent(manifest, instanceName),
			Permissions: "0644",
			Owner:       "root:root",
		})
	}
	cc.WriteFiles = append(cc.WriteFiles, serialConsoleFiles(manifest, family)...)
	for _, f := range manifest.CloudInit.WriteFiles {
		cc.WriteFiles = append(cc.WriteFiles, ccFile{
			Path:        f.Path,
			Content:     f.Content,
			Permissions: f.Permissions,
			Owner:       f.Owner,
		})
	}

	cc.BootCmd = manifest.CloudInit.BootCmd

	cc.RunCmd = append(manifest.CloudInit.RunCmd, serialConsoleRunCmd(family)...)
	cc.RunCmd = append(cc.RunCmd, volumeMountRunCmd(manifest)...)

	data, _ := yaml.Marshal(cc)
	ud := "#cloud-config\n" + string(data)

	md := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceName, hostname(manifest, instanceName))

	var nc string
	if manifest.InternalNetwork != nil {
		nc = renderNetworkConfig(manifest, instanceIndex)
	}

	return ud, md, nc
}

// osFamilyFromMetadata maps the explicit image metadata resolved from the
// registry or compose file into cloud-init rendering behavior. Unknown or empty
// metadata stays systemd for compatibility with existing local images.
func osFamilyFromMetadata(imageOS string) osFamily {
	if imageOS == config.ImageOSOpenRC {
		return familyOpenRC
	}
	return familySystemd
}

// renderUser builds the cloud-config users[0] entry. On systemd distros we
// set shell, groups, and sudo explicitly (matching existing behavior); on
// Alpine we omit those because /bin/bash, the "adm"/"sudo" groups, and the
// sudo binary are not present in the default cloud image.
func renderUser(manifest config.Manifest, family osFamily) ccUser {
	user := ccUser{
		Name:              manifest.CloudInit.User,
		SSHAuthorizedKeys: manifest.CloudInit.SSHAuthorizedKeys,
	}
	switch family {
	case familySystemd:
		user.Groups = []string{"adm", "sudo"}
		user.Shell = "/bin/bash"
		user.Sudo = "ALL=(ALL) NOPASSWD:ALL"
	case familyOpenRC:
		// Leave defaults to cloud-init / tiny-cloud; /bin/sh is guaranteed.
		user.Shell = "/bin/sh"
	}
	return user
}

// serialConsoleFiles returns distro-specific write_files needed to land on a
// usable serial console. On systemd we add a GRUB drop-in and a serial-getty
// autologin override. On OpenRC (Alpine) the cloud image already exposes
// ttyS0, so there is nothing to write.
func serialConsoleFiles(manifest config.Manifest, family osFamily) []ccFile {
	if family != familySystemd {
		return nil
	}
	return []ccFile{
		{
			Path:        "/etc/default/grub.d/99-serial-console.cfg",
			Content:     "GRUB_CMDLINE_LINUX_DEFAULT=\"${GRUB_CMDLINE_LINUX_DEFAULT} console=ttyS0,115200\"\nGRUB_TERMINAL=\"serial console\"\nGRUB_SERIAL_COMMAND=\"serial --speed=115200\"\n",
			Permissions: "0644",
			Owner:       "root:root",
		},
		{
			Path:        "/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
			Content:     fmt.Sprintf("[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin %s --noclear %%I $TERM\n", manifest.CloudInit.User),
			Permissions: "0644",
			Owner:       "root:root",
		},
	}
}

// serialConsoleRunCmd returns runcmd entries needed to activate the serial
// console on first boot. On Alpine the cloud image already spawns a getty on
// ttyS0, so no command is required.
func serialConsoleRunCmd(family osFamily) []string {
	if family != familySystemd {
		return nil
	}
	return []string{serialGettySystemdCmd}
}

func renderNetworkConfig(manifest config.Manifest, instanceIndex int) string {
	ip := manifest.InternalNetwork.InstanceIP(instanceIndex)
	mac := manifest.InternalNetwork.InstanceMAC(instanceIndex)
	if ip == "" || mac == "" {
		return ""
	}

	ethernets := map[string]ethernetDef{
		"internal": {
			Match:     matchDef{MACAddress: mac},
			Addresses: []string{ip + "/24"},
		},
	}

	if userMAC := manifest.InternalNetwork.UserMAC(instanceIndex); userMAC != "" {
		ethernets["external"] = ethernetDef{
			Match: matchDef{MACAddress: userMAC},
			DHCP4: true,
		}
	}

	nc := netConfig{Network: netConfigBody{
		Version:   2,
		Ethernets: ethernets,
	}}

	data, _ := yaml.Marshal(nc)
	return string(data)
}

// volumeMountRunCmd produces a runcmd snippet per named volume that
// runs on every boot but is idempotent: mkfs only when the block device
// has no detectable filesystem, fstab edit only on first hit, and
// `mount -a` at the end so an empty fstab doesn't fail a reconcile.
//
// We use /dev/disk/by-id/virtio-<serial> because the PCI device number
// (and thus /dev/vdX naming) changes with any hardware layout tweak;
// the by-id path is stable across reboots and virtual-hardware edits.
func volumeMountRunCmd(manifest config.Manifest) []string {
	var cmds []string
	for _, m := range manifest.Mounts {
		if m.Kind != config.MountKindVolume {
			continue
		}
		dev := "/dev/disk/by-id/virtio-vol-" + m.VolumeName
		target := m.Target
		label := "vol-" + m.VolumeName
		// Quote embedded targets defensively; most will be plain paths
		// but users can put spaces anywhere.

		// Read-only volumes can't be formatted or have their fstab
		// entry written by the guest: the QEMU drive is opened
		// readonly=on, so mkfs.ext4 would fail and every fresh boot
		// would dump a noisy error into cloud-init. More importantly,
		// an `ro`-in-compose contract must hold end-to-end: we must
		// not let the guest mount a volume rw when the operator
		// asked for ro, even if the filesystem is technically
		// writable in a future rebuild. Split the fstab/mount path
		// on m.ReadOnly so ro volumes get `ro,nofail` in fstab and
		// skip mkfs entirely (assuming the volume was pre-populated
		// by a prior rw run or by an operator).
		var steps []string
		steps = append(steps,
			fmt.Sprintf("udevadm settle --exit-if-exists=%s || true", shquote(dev)),
		)
		if !m.ReadOnly {
			steps = append(steps,
				fmt.Sprintf("if [ -b %s ] && ! blkid %s >/dev/null 2>&1; then mkfs.ext4 -F -L %s %s; fi",
					shquote(dev), shquote(dev), shquote(label), shquote(dev)),
			)
		}
		steps = append(steps, fmt.Sprintf("mkdir -p %s", shquote(target)))

		fstabOpts := "defaults,nofail"
		if m.ReadOnly {
			fstabOpts = "ro,nofail"
		}
		steps = append(steps,
			fmt.Sprintf("grep -qE %s /etc/fstab || echo %s >> /etc/fstab",
				shquote(" "+target+" "),
				shquote(dev+" "+target+" ext4 "+fstabOpts+" 0 2"),
			),
			fmt.Sprintf("mountpoint -q %s || mount %s || { echo %s >&2; exit 1; }",
				shquote(target), shquote(target), shquote("holos: failed to mount volume "+m.VolumeName+" at "+target)),
		)
		cmds = append(cmds, strings.Join(steps, " && "))
	}
	return cmds
}

// shquote wraps s in single quotes and escapes any embedded single
// quotes by ending the quoted region, inserting an escaped single
// quote, and reopening. The only reliable way to embed a quote in
// a single-quoted POSIX shell string.
func shquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func hostname(manifest config.Manifest, instanceName string) string {
	if manifest.CloudInit.Hostname != "" {
		return manifest.CloudInit.Hostname
	}
	return instanceName
}

func hostsFileContent(manifest config.Manifest, instanceName string) string {
	var buf strings.Builder
	buf.WriteString("127.0.0.1 localhost\n")
	fmt.Fprintf(&buf, "127.0.1.1 %s\n", instanceName)
	buf.WriteString("::1 localhost ip6-localhost ip6-loopback\n")
	buf.WriteString("ff02::1 ip6-allnodes\n")
	buf.WriteString("ff02::2 ip6-allrouters\n")
	buf.WriteString("\n")

	ipToHosts := make(map[string][]string)
	for host, ip := range manifest.ExtraHosts {
		ipToHosts[ip] = append(ipToHosts[ip], host)
	}

	var ips []string
	for ip := range ipToHosts {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	for _, ip := range ips {
		names := ipToHosts[ip]
		sort.Strings(names)
		fmt.Fprintf(&buf, "%s %s\n", ip, strings.Join(names, " "))
	}

	return buf.String()
}
