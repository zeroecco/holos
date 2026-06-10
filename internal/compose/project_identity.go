package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const defaultNetworkName = "default"

// planNetwork derives multicast groups and ports for a project's internal
// networks from SHA-256. Using a cryptographic hash across three group octets
// and the port gives ~40 bits of entropy per segment, which makes accidental
// collisions between unrelated stacks on the same host vanishingly unlikely.
//
// The group is drawn from the IPv4 administratively-scoped range
// 239.0.0.0/8 (RFC 2365), which is intended for local use and is not
// forwarded outside the host.
func (f *File) planNetwork() NetworkPlan {
	names := make([]string, 0, len(f.Networks))
	for name := range f.Networks {
		if name != defaultNetworkName {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	segments := make(map[string]NetworkSegmentPlan, len(names)+1)
	segments[defaultNetworkName] = planNetworkSegment(f.Name, defaultNetworkName, 0)
	for i, name := range names {
		segments[name] = planNetworkSegment(f.Name, name, i+1)
	}
	primary := segments[defaultNetworkName]

	return NetworkPlan{
		MulticastGroup: primary.MulticastGroup,
		MulticastPort:  primary.MulticastPort,
		Subnet:         primary.Subnet,
		Segments:       segments,
	}
}

func planNetworkSegment(project, name string, index int) NetworkSegmentPlan {
	sum := sha256.Sum256([]byte(project + "/" + name))

	group := fmt.Sprintf("239.%d.%d.%d", sum[0], sum[1], sum[2])
	portBase := uint16(sum[3])<<8 | uint16(sum[4])
	port := 10000 + int(portBase)%55000

	return NetworkSegmentPlan{
		Name:           name,
		MulticastGroup: group,
		MulticastPort:  port,
		Subnet:         fmt.Sprintf("10.10.%d.0/24", index),
	}
}

func (f *File) specHash() (string, error) {
	data, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// generateMAC produces a locally-administered unicast MAC derived from the
// SHA-256 of the project and service names. The layout is:
//
//	52:54:<prefix>:<h0>:<h1>:00
//
// where prefix distinguishes the internal NIC (0x00) from the user NIC
// (0x01), h0/h1 are two bytes of SHA-256 entropy, and the last octet is
// reserved for the per-replica offset applied by InstanceMAC.
//
// Cross-project MAC collision risk is bounded by the multicast
// group+port pair (~40 bits of entropy): two VMs only share an L2 segment
// when their projects collide in BOTH group and port.
func generateMAC(prefix byte, project, service string) string {
	sum := sha256.Sum256([]byte(project + "/" + service))
	return fmt.Sprintf("52:54:%02x:%02x:%02x:00", prefix, sum[0], sum[1])
}

func generateNetworkMAC(project, service, network string) string {
	sum := sha256.Sum256([]byte(project + "/" + service + "/" + network))
	return fmt.Sprintf("52:54:02:%02x:%02x:00", sum[0], sum[1])
}
