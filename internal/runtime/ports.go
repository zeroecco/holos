package runtime

import (
	"errors"
	"fmt"
	"net"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

func allocatePorts(manifest config.Manifest, index int) ([]qemu.PortMapping, error) {
	mappings := make([]qemu.PortMapping, 0, len(manifest.Ports))
	for _, port := range manifest.Ports {
		hostPort := port.HostPort
		if hostPort > 0 {
			hostPort += index
			if err := ensureTCPPortAvailable(hostPort); err != nil {
				return nil, err
			}
		} else {
			allocated, err := allocateEphemeralTCPPort()
			if err != nil {
				return nil, err
			}
			hostPort = allocated
		}

		mappings = append(mappings, qemu.PortMapping{
			Name:      port.Name,
			HostPort:  hostPort,
			GuestPort: port.GuestPort,
			Protocol:  port.Protocol,
		})
	}
	return mappings, nil
}

func ensureTCPPortAvailable(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("host port %d is unavailable: %w", port, err)
	}
	return listener.Close()
}

func allocateEphemeralTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate ephemeral port: %w", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected tcp listener address type")
	}
	return address.Port, nil
}
