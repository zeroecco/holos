package cloudinit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	hostsFilePath        = "/etc/hosts"
	hostsNameSeparator   = " "
	loopbackHostLine     = "127.0.0.1 localhost"
	instanceHostAddr     = "127.0.1.1"
	ipv6LoopbackHostLine = "::1 localhost ip6-localhost ip6-loopback"
	ipv6AllNodesLine     = "ff02::1 ip6-allnodes"
	ipv6AllRoutersLine   = "ff02::2 ip6-allrouters"
)

func hostname(manifest config.Manifest, instanceName string) string {
	if manifest.CloudInit.Hostname != "" {
		return manifest.CloudInit.Hostname
	}
	return instanceName
}

func hostsFileContent(manifest config.Manifest, instanceName string) string {
	var buf strings.Builder
	fmt.Fprintln(&buf, loopbackHostLine)
	fmt.Fprintf(&buf, "%s %s\n", instanceHostAddr, instanceName)
	fmt.Fprintln(&buf, ipv6LoopbackHostLine)
	fmt.Fprintln(&buf, ipv6AllNodesLine)
	fmt.Fprintln(&buf, ipv6AllRoutersLine)
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
		fmt.Fprintf(&buf, "%s %s\n", ip, strings.Join(names, hostsNameSeparator))
	}

	return buf.String()
}

func hostsWriteFile(manifest config.Manifest, instanceName string) (ccFile, bool) {
	if len(manifest.ExtraHosts) == 0 {
		return ccFile{}, false
	}
	return ccFile{
		Path:        hostsFilePath,
		Content:     hostsFileContent(manifest, instanceName),
		Permissions: config.DefaultFilePermissions,
		Owner:       config.DefaultFileOwner,
	}, true
}
