package cloudinit

import (
	"github.com/zeroecco/holos/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	networkConfigVersion       = 2
	internalNetworkInterface   = "internal"
	externalNetworkInterface   = "external"
	internalNetworkAddressCIDR = "/24"
)

func renderNetworkConfig(manifest config.Manifest, instanceIndex int) string {
	mac := manifest.InternalNetwork.InstanceMAC(instanceIndex)
	if mac == "" {
		return ""
	}
	primaryIP := manifest.InternalNetwork.InstanceIP(instanceIndex)
	primaryDHCP := manifest.InternalNetwork.Backend == "bridge"
	if primaryIP == "" && !primaryDHCP {
		return ""
	}

	ethernets := map[string]ethernetDef{
		internalNetworkInterface: internalEthernetDef(
			mac,
			primaryIP,
			primaryDHCP,
		),
	}
	if len(manifest.InternalNetwork.DNSSearch) > 0 {
		internal := ethernets[internalNetworkInterface]
		internal.Nameservers = &nameserverDef{
			Search: append([]string(nil), manifest.InternalNetwork.DNSSearch...),
		}
		ethernets[internalNetworkInterface] = internal
	}

	if userMAC := manifest.InternalNetwork.UserMAC(instanceIndex); userMAC != "" {
		ethernets[externalNetworkInterface] = ethernetDef{
			Match: matchDef{MACAddress: userMAC},
			DHCP4: true,
		}
	}
	for _, segment := range manifest.InternalNetwork.Segments {
		ip := segment.SegmentIP(instanceIndex)
		mac := segment.SegmentMAC(instanceIndex)
		dhcp := segment.Backend == "bridge"
		if mac == "" || (ip == "" && !dhcp) {
			continue
		}
		ethernets[segmentInterfaceName(segment.Name)] = internalEthernetDef(
			mac,
			ip,
			dhcp,
		)
	}

	nc := netConfig{Network: netConfigBody{
		Version:   networkConfigVersion,
		Ethernets: ethernets,
	}}

	data, _ := yaml.Marshal(nc)
	return string(data)
}

func segmentInterfaceName(name string) string {
	return "internal-" + name
}

func internalEthernetDef(mac string, ip string, dhcp bool) ethernetDef {
	def := ethernetDef{Match: matchDef{MACAddress: mac}}
	if dhcp {
		def.DHCP4 = true
		return def
	}
	def.Addresses = []string{ip + internalNetworkAddressCIDR}
	return def
}
