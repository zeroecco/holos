package dockerfile

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zeroecco/holos/internal/config"
)

const (
	exposeNameFormat = "expose-%d"
	exposeMinPort    = 1
	exposeMaxPort    = 65535
)

func parseExpose(args string) ([]config.PortForward, error) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return nil, fmt.Errorf("requires at least one port")
	}
	ports := make([]config.PortForward, 0, len(fields))
	for _, field := range fields {
		port, protocol, err := parseExposePort(field)
		if err != nil {
			return nil, err
		}
		ports = append(ports, config.PortForward{
			HostPort:  0,
			GuestPort: port,
			Protocol:  protocol,
		})
	}
	return ports, nil
}

func parseExposePort(raw string) (int, string, error) {
	portText, protocol, found := strings.Cut(raw, "/")
	if !found {
		protocol = config.DefaultProtocol
	} else {
		protocol = strings.ToLower(protocol)
	}
	if portText == "" {
		return 0, "", fmt.Errorf("port %q is missing a port number", raw)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < exposeMinPort || port > exposeMaxPort {
		return 0, "", fmt.Errorf("port %q is out of range", raw)
	}
	if err := config.ValidatePortProtocol(protocol); err != nil {
		return 0, "", err
	}
	return port, protocol, nil
}

func exposeName(index int) string {
	return fmt.Sprintf(exposeNameFormat, index)
}
