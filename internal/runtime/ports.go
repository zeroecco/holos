package runtime

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	tcpNetwork        = "tcp"
	ephemeralPortSpec = "0"
)

func allocatePorts(manifest config.Manifest, index int) ([]qemu.PortMapping, error) {
	mappings := make([]qemu.PortMapping, 0, len(manifest.Ports))
	for _, port := range manifest.Ports {
		hostAddr := effectiveHostAddr(port.HostAddr)
		hostPort := port.HostPort
		if hostPort > 0 {
			hostPort += index
			if err := ensureTCPPortAvailable(hostAddr, hostPort); err != nil {
				return nil, err
			}
		} else {
			allocated, err := allocateEphemeralTCPPortOn(hostAddr)
			if err != nil {
				return nil, err
			}
			hostPort = allocated
		}

		mappings = append(mappings, qemuPortMapping(port, hostAddr, hostPort))
	}
	return mappings, nil
}

func qemuPortMapping(port config.PortForward, hostAddr string, hostPort int) qemu.PortMapping {
	return qemu.PortMapping{
		Name:      port.Name,
		HostAddr:  hostAddr,
		HostPort:  hostPort,
		GuestAddr: port.GuestAddr,
		GuestPort: port.GuestPort,
		Protocol:  port.Protocol,
	}
}

func ensureTCPPortAvailable(addr string, port int) error {
	listener, err := net.Listen(tcpNetwork, net.JoinHostPort(addr, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("host port %s:%d is unavailable: %w", addr, port, err)
	}
	return listener.Close()
}

func allocateEphemeralTCPPort() (int, error) {
	return allocateEphemeralTCPPortOn(defaultHostAddr)
}

func allocateEphemeralTCPPortOn(addr string) (int, error) {
	if port, ok, err := nextTestEphemeralTCPPort(); ok || err != nil {
		return port, err
	}

	listener, err := net.Listen(tcpNetwork, net.JoinHostPort(addr, ephemeralPortSpec))
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

func effectiveHostAddr(addr string) string {
	if addr == "" {
		return defaultHostAddr
	}
	return addr
}
