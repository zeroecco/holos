package qemu

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

func networkArgs(manifest config.Manifest, spec LaunchSpec, userNetdev string) []string {
	userDevice := userNetDevice(manifest.InternalNetwork, spec.Index)
	args := []string{"-netdev", userNetdev, qemuArgDevice, userDevice}
	if manifest.InternalNetwork == nil {
		return args
	}

	mac := manifest.InternalNetwork.InstanceMAC(spec.Index)
	args = append(args,
		"-netdev", internalNetdev(socketNetdevID, manifest.InternalNetwork),
		qemuArgDevice, virtioNetDevice(socketNetdevID, mac))
	for i, segment := range manifest.InternalNetwork.Segments {
		netdevID := segmentNetdevID(i)
		args = append(args,
			"-netdev", segmentNetdev(netdevID, segment),
			qemuArgDevice, virtioNetDevice(netdevID, segment.SegmentMAC(spec.Index)))
	}
	return args
}

func userNetDevice(network *config.InternalNetworkConfig, index int) string {
	if network == nil {
		return virtioNetDevice(userNetdevID, "")
	}
	return virtioNetDevice(userNetdevID, network.UserMAC(index))
}

func socketNetdev(network *config.InternalNetworkConfig) string {
	return fmt.Sprintf("socket,id=%s,mcast=%s",
		socketNetdevID,
		socketMulticastTarget(network))
}

func internalNetdev(netdevID string, network *config.InternalNetworkConfig) string {
	if network.Backend == bridgeBackend && network.BridgeName != "" {
		return bridgeNetdev(netdevID, network.BridgeName)
	}
	return fmt.Sprintf("socket,id=%s,mcast=%s", netdevID, socketMulticastTarget(network))
}

func socketMulticastTarget(network *config.InternalNetworkConfig) string {
	return fmt.Sprintf("%s:%d", network.MulticastGroup, network.MulticastPort)
}

func segmentNetdevID(index int) string {
	return fmt.Sprintf("net%d", index+2)
}

func socketSegmentNetdev(netdevID string, segment config.InternalNetworkSegment) string {
	return fmt.Sprintf("socket,id=%s,mcast=%s:%d",
		netdevID,
		segment.MulticastGroup,
		segment.MulticastPort)
}

func segmentNetdev(netdevID string, segment config.InternalNetworkSegment) string {
	if segment.Backend == bridgeBackend && segment.BridgeName != "" {
		return bridgeNetdev(netdevID, segment.BridgeName)
	}
	return socketSegmentNetdev(netdevID, segment)
}

func bridgeNetdev(netdevID string, bridgeName string) string {
	return qemuOptions("bridge", qemuKeyValue("id", netdevID), qemuKeyValue("br", bridgeName))
}

func virtioNetDevice(netdevID, mac string) string {
	options := []string{"virtio-net-pci", qemuKeyValue(netdevKey, netdevID)}
	if mac != "" {
		options = append(options, qemuKeyValue(macKey, mac))
	}
	return qemuOptions(options...)
}
