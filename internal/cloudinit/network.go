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
	ip := manifest.InternalNetwork.InstanceIP(instanceIndex)
	mac := manifest.InternalNetwork.InstanceMAC(instanceIndex)
	if ip == "" || mac == "" {
		return ""
	}

	ethernets := map[string]ethernetDef{
		internalNetworkInterface: {
			Match:     matchDef{MACAddress: mac},
			Addresses: []string{ip + internalNetworkAddressCIDR},
		},
	}

	if userMAC := manifest.InternalNetwork.UserMAC(instanceIndex); userMAC != "" {
		ethernets[externalNetworkInterface] = ethernetDef{
			Match: matchDef{MACAddress: userMAC},
			DHCP4: true,
		}
	}

	nc := netConfig{Network: netConfigBody{
		Version:   networkConfigVersion,
		Ethernets: ethernets,
	}}

	data, _ := yaml.Marshal(nc)
	return string(data)
}
