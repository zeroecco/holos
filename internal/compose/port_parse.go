package compose

import (
	"fmt"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const portProtocolSeparator = "/"

const (
	portSpecGuestOnlyParts         = 1
	portSpecHostGuestParts         = 2
	portSpecHostAddrGuestParts     = 3
	portSpecHostAddrGuestAddrParts = 4
)

func parsePort(spec string) ([]config.PortForward, error) {
	spec, protocol, err := splitPortProtocol(spec)
	if err != nil {
		return nil, err
	}

	parts, err := splitPortSpec(spec)
	if err != nil {
		return nil, err
	}
	switch len(parts) {
	case portSpecGuestOnlyParts:
		guests, err := parseGuestOnlyPortRange(parts[0])
		if err != nil {
			return nil, err
		}
		return guestOnlyPortForwards(guests, protocol), nil
	case portSpecHostGuestParts:
		hosts, err := parseHostPortRange(parts[0])
		if err != nil {
			return nil, err
		}
		guests, err := parseGuestPortRange(parts[1])
		if err != nil {
			return nil, err
		}
		return expandPortRanges(hosts, guests, "", "", protocol)
	case portSpecHostAddrGuestParts:
		hostAddr, err := parsePortAddress("host", parts[0])
		if err != nil {
			return nil, err
		}
		hosts, err := parseHostPortRange(parts[1])
		if err != nil {
			return nil, err
		}
		guests, err := parseGuestPortRange(parts[2])
		if err != nil {
			return nil, err
		}
		return expandPortRanges(hosts, guests, hostAddr, "", protocol)
	case portSpecHostAddrGuestAddrParts:
		hostAddr, err := parsePortAddress("host", parts[0])
		if err != nil {
			return nil, err
		}
		hosts, err := parseHostPortRange(parts[1])
		if err != nil {
			return nil, err
		}
		guestAddr, err := parsePortAddress("guest", parts[2])
		if err != nil {
			return nil, err
		}
		guests, err := parseGuestPortRange(parts[3])
		if err != nil {
			return nil, err
		}
		return expandPortRanges(hosts, guests, hostAddr, guestAddr, protocol)
	default:
		return nil, fmt.Errorf(invalidPortSpecError)
	}
}

func splitPortProtocol(spec string) (string, string, error) {
	protocol := ""
	if idx := strings.LastIndex(spec, portProtocolSeparator); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}
	protocol, err := normalizePortProtocol(protocol)
	if err != nil {
		return "", "", err
	}
	return spec, protocol, nil
}

func guestOnlyPortForwards(guests []int, protocol string) []config.PortForward {
	out := make([]config.PortForward, 0, len(guests))
	for _, guest := range guests {
		out = append(out, config.PortForward{GuestPort: guest, Protocol: protocol})
	}
	return out
}

func parseGuestOnlyPortRange(raw string) ([]int, error) {
	ports, err := parsePortRange(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}
	return ports, nil
}

func parseHostPortRange(raw string) ([]int, error) {
	ports, err := parsePortRange(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid host port: %w", err)
	}
	return ports, nil
}

func parseGuestPortRange(raw string) ([]int, error) {
	ports, err := parsePortRange(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid guest port: %w", err)
	}
	return ports, nil
}

func normalizePortProtocol(protocol string) (string, error) {
	if protocol == "" {
		protocol = config.DefaultProtocol
	}
	// Only TCP forwarding is implemented end-to-end; reject other
	// protocols at parse time rather than let the user discover the
	// limitation at `holos up` via a validation error.
	if protocol != config.DefaultProtocol {
		return "", fmt.Errorf("protocol %q is unsupported; only %s is implemented", protocol, config.DefaultProtocol)
	}
	return protocol, nil
}

func expandPortRanges(hosts, guests []int, hostAddr, guestAddr, protocol string) ([]config.PortForward, error) {
	if len(hosts) != len(guests) {
		return nil, fmt.Errorf("host and guest port ranges must have the same length")
	}
	out := make([]config.PortForward, 0, len(hosts))
	for i := range hosts {
		out = append(out, rangePortForward(hosts[i], guests[i], hostAddr, guestAddr, protocol))
	}
	return out, nil
}

func rangePortForward(hostPort, guestPort int, hostAddr, guestAddr, protocol string) config.PortForward {
	return config.PortForward{
		HostAddr:  hostAddr,
		HostPort:  hostPort,
		GuestAddr: guestAddr,
		GuestPort: guestPort,
		Protocol:  protocol,
	}
}
