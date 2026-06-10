package compose

import (
	"fmt"

	"github.com/zeroecco/holos/internal/config"
)

const (
	generatedSinglePortNameFormat = "port-%d"
	generatedRangePortNameFormat  = "port-%d-%d"
	ephemeralHostPort             = 0
	portModeHost                  = "host"
	portModeIngress               = "ingress"
)

func parsePorts(specs []ComposePort) ([]config.PortForward, error) {
	ports := make([]config.PortForward, 0, len(specs))
	for i, spec := range specs {
		parsed, err := parseComposePort(spec)
		if err != nil {
			return nil, fmt.Errorf("port %d: %w", i, err)
		}
		for j := range parsed {
			if parsed[j].Name == "" {
				parsed[j].Name = generatedPortName(i, j, len(parsed))
			}
			ports = append(ports, parsed[j])
		}
	}
	return ports, nil
}

func generatedPortName(specIndex, parsedIndex, parsedCount int) string {
	if parsedCount == 1 {
		return fmt.Sprintf(generatedSinglePortNameFormat, specIndex)
	}
	return fmt.Sprintf(generatedRangePortNameFormat, specIndex, parsedIndex)
}

func parseComposePort(port ComposePort) ([]config.PortForward, error) {
	if port.Short != "" {
		return parsePort(port.Short)
	}
	if !port.hasTarget && port.Target == 0 {
		return nil, fmt.Errorf("target is required")
	}
	protocol, err := normalizePortProtocol(port.Protocol)
	if err != nil {
		return nil, err
	}
	if err := validatePortMode(port.Mode); err != nil {
		return nil, err
	}

	var hostAddr string
	if port.HostIP != "" {
		addr, err := parsePortAddress("host", port.HostIP)
		if err != nil {
			return nil, err
		}
		hostAddr = addr
	}

	hostPorts, err := composePortHostPorts(port.Published)
	if err != nil {
		return nil, err
	}

	out := make([]config.PortForward, 0, len(hostPorts))
	for _, hostPort := range hostPorts {
		out = append(out, composePortForward(port, hostAddr, hostPort, protocol))
	}
	return out, nil
}

func composePortHostPorts(published string) ([]int, error) {
	if published == "" {
		return []int{ephemeralHostPort}, nil
	}
	parsed, err := parsePortRange(published)
	if err != nil {
		return nil, fmt.Errorf("invalid published port: %w", err)
	}
	return parsed, nil
}

func composePortForward(port ComposePort, hostAddr string, hostPort int, protocol string) config.PortForward {
	return config.PortForward{
		Name:      port.Name,
		HostAddr:  hostAddr,
		HostPort:  hostPort,
		GuestPort: port.Target,
		Protocol:  protocol,
	}
}

func validatePortMode(mode string) error {
	switch mode {
	case "", portModeHost, portModeIngress:
		return nil
	default:
		return fmt.Errorf("mode %q is unsupported; expected host or ingress", mode)
	}
}
