package cloudinit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

// Render produces cloud-init user-data, meta-data, and network-config.
// networkConfig is empty when there is no internal network.
func Render(manifest config.Manifest, instanceName string, instanceIndex int) (userData, metaData, networkConfig string) {
	var ud strings.Builder
	ud.WriteString("#cloud-config\n")
	ud.WriteString(fmt.Sprintf("hostname: %s\n", yamlQuote(hostname(manifest, instanceName))))

	if len(manifest.ExtraHosts) > 0 {
		ud.WriteString("manage_etc_hosts: false\n")
	} else {
		ud.WriteString("manage_etc_hosts: true\n")
	}
	ud.WriteString("ssh_pwauth: false\n")

	if len(manifest.CloudInit.Packages) > 0 {
		ud.WriteString("package_update: true\n")
		ud.WriteString("packages:\n")
		for _, pkg := range manifest.CloudInit.Packages {
			ud.WriteString(fmt.Sprintf("  - %s\n", yamlQuote(pkg)))
		}
	}

	ud.WriteString("users:\n")
	ud.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(manifest.CloudInit.User)))
	ud.WriteString("    groups: [adm, sudo]\n")
	ud.WriteString("    shell: /bin/bash\n")
	ud.WriteString(fmt.Sprintf("    sudo: %s\n", yamlQuote("ALL=(ALL) NOPASSWD:ALL")))
	if len(manifest.CloudInit.SSHAuthorizedKeys) > 0 {
		ud.WriteString("    ssh_authorized_keys:\n")
		for _, key := range manifest.CloudInit.SSHAuthorizedKeys {
			ud.WriteString(fmt.Sprintf("      - %s\n", yamlQuote(key)))
		}
	}

	// Merge system write_files (hosts, GRUB serial console) with user write_files.
	var allWriteFiles []config.WriteFile
	if len(manifest.ExtraHosts) > 0 {
		allWriteFiles = append(allWriteFiles, config.WriteFile{
			Path:        "/etc/hosts",
			Content:     hostsFileContent(manifest, instanceName),
			Permissions: "0644",
			Owner:       "root:root",
		})
	}
	allWriteFiles = append(allWriteFiles, config.WriteFile{
		Path:        "/etc/default/grub.d/99-serial-console.cfg",
		Content:     "GRUB_CMDLINE_LINUX_DEFAULT=\"${GRUB_CMDLINE_LINUX_DEFAULT} console=ttyS0,115200\"\nGRUB_TERMINAL=\"serial console\"\nGRUB_SERIAL_COMMAND=\"serial --speed=115200\"\n",
		Permissions: "0644",
		Owner:       "root:root",
	})
	allWriteFiles = append(allWriteFiles, manifest.CloudInit.WriteFiles...)

	ud.WriteString("write_files:\n")
	for _, file := range allWriteFiles {
		ud.WriteString(fmt.Sprintf("  - path: %s\n", yamlQuote(file.Path)))
		ud.WriteString(fmt.Sprintf("    owner: %s\n", yamlQuote(file.Owner)))
		ud.WriteString(fmt.Sprintf("    permissions: %s\n", yamlQuote(file.Permissions)))
		ud.WriteString("    content: |\n")
		ud.WriteString(indentBlock(file.Content, "      "))
	}

	if len(manifest.CloudInit.BootCmd) > 0 {
		ud.WriteString("bootcmd:\n")
		for _, command := range manifest.CloudInit.BootCmd {
			ud.WriteString(fmt.Sprintf("  - %s\n", yamlQuote(command)))
		}
	}

	serialGettyCmd := "systemctl enable --now serial-getty@ttyS0.service && update-grub 2>/dev/null || true"
	var runCmds []string
	runCmds = append(runCmds, serialGettyCmd)
	runCmds = append(runCmds, manifest.CloudInit.RunCmd...)
	ud.WriteString("runcmd:\n")
	for _, command := range runCmds {
		ud.WriteString(fmt.Sprintf("  - %s\n", yamlQuote(command)))
	}

	md := fmt.Sprintf(
		"instance-id: %s\nlocal-hostname: %s\n",
		instanceName,
		hostname(manifest, instanceName),
	)

	var nc string
	if manifest.InternalNetwork != nil {
		nc = renderNetworkConfig(manifest, instanceIndex)
	}

	return ud.String(), md, nc
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
	buf.WriteString(fmt.Sprintf("127.0.1.1 %s\n", instanceName))
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
		buf.WriteString(fmt.Sprintf("%s %s\n", ip, strings.Join(names, " ")))
	}

	return buf.String()
}

func renderNetworkConfig(manifest config.Manifest, instanceIndex int) string {
	ip := manifest.InternalNetwork.InstanceIP(instanceIndex)
	mac := manifest.InternalNetwork.InstanceMAC(instanceIndex)
	if ip == "" || mac == "" {
		return ""
	}

	userMAC := manifest.InternalNetwork.UserMAC(instanceIndex)

	var buf strings.Builder
	buf.WriteString("network:\n  version: 2\n  ethernets:\n")

	if userMAC != "" {
		fmt.Fprintf(&buf, "    external:\n      match:\n        macaddress: %q\n      dhcp4: true\n", userMAC)
	}

	fmt.Fprintf(&buf, "    internal:\n      match:\n        macaddress: %q\n      dhcp4: false\n      addresses:\n        - %s/24\n", mac, ip)

	return buf.String()
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	if len(lines) == 0 {
		return prefix + "\n"
	}

	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(prefix)
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func yamlQuote(value string) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}
