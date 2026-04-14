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
	Groups            []string `yaml:"groups"`
	Shell             string   `yaml:"shell"`
	Sudo              string   `yaml:"sudo"`
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

const serialGettyCmd = "systemctl daemon-reload && systemctl enable serial-getty@ttyS0.service && systemctl restart serial-getty@ttyS0.service && update-grub 2>/dev/null || true"

// Render produces cloud-init user-data, meta-data, and network-config.
// networkConfig is empty when there is no internal network.
func Render(manifest config.Manifest, instanceName string, instanceIndex int) (userData, metaData, networkConfig string) {
	cc := cloudConfig{
		Hostname:       hostname(manifest, instanceName),
		ManageEtcHosts: len(manifest.ExtraHosts) == 0,
		Users: []ccUser{{
			Name:              manifest.CloudInit.User,
			Groups:            []string{"adm", "sudo"},
			Shell:             "/bin/bash",
			Sudo:              "ALL=(ALL) NOPASSWD:ALL",
			SSHAuthorizedKeys: manifest.CloudInit.SSHAuthorizedKeys,
		}},
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
	cc.WriteFiles = append(cc.WriteFiles,
		ccFile{
			Path:        "/etc/default/grub.d/99-serial-console.cfg",
			Content:     "GRUB_CMDLINE_LINUX_DEFAULT=\"${GRUB_CMDLINE_LINUX_DEFAULT} console=ttyS0,115200\"\nGRUB_TERMINAL=\"serial console\"\nGRUB_SERIAL_COMMAND=\"serial --speed=115200\"\n",
			Permissions: "0644",
			Owner:       "root:root",
		},
		ccFile{
			Path:        "/etc/systemd/system/serial-getty@ttyS0.service.d/autologin.conf",
			Content:     fmt.Sprintf("[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin %s --noclear %%I $TERM\n", manifest.CloudInit.User),
			Permissions: "0644",
			Owner:       "root:root",
		},
	)
	for _, f := range manifest.CloudInit.WriteFiles {
		cc.WriteFiles = append(cc.WriteFiles, ccFile{
			Path:        f.Path,
			Content:     f.Content,
			Permissions: f.Permissions,
			Owner:       f.Owner,
		})
	}

	cc.BootCmd = manifest.CloudInit.BootCmd

	cc.RunCmd = append([]string{serialGettyCmd}, manifest.CloudInit.RunCmd...)

	data, _ := yaml.Marshal(cc)
	ud := "#cloud-config\n" + string(data)

	md := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", instanceName, hostname(manifest, instanceName))

	var nc string
	if manifest.InternalNetwork != nil {
		nc = renderNetworkConfig(manifest, instanceIndex)
	}

	return ud, md, nc
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
