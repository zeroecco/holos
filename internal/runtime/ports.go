package runtime

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

var (
	testEphemeralPortsMu    sync.Mutex
	testEphemeralPortsValue string
	testEphemeralPortsIndex int
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
	if port, ok, err := nextTestEphemeralTCPPort(); ok || err != nil {
		return port, err
	}

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

func nextTestEphemeralTCPPort() (int, bool, error) {
	raw := os.Getenv("HOLOS_TEST_EPHEMERAL_PORTS")
	if raw == "" {
		return 0, false, nil
	}

	testEphemeralPortsMu.Lock()
	defer testEphemeralPortsMu.Unlock()

	if raw != testEphemeralPortsValue {
		testEphemeralPortsValue = raw
		testEphemeralPortsIndex = 0
	}

	parts := strings.Split(raw, ",")
	if testEphemeralPortsIndex >= len(parts) {
		return 0, true, fmt.Errorf("HOLOS_TEST_EPHEMERAL_PORTS exhausted after %d allocations", len(parts))
	}
	value := strings.TrimSpace(parts[testEphemeralPortsIndex])
	testEphemeralPortsIndex++

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, true, fmt.Errorf("invalid HOLOS_TEST_EPHEMERAL_PORTS entry %q", value)
	}
	return port, true, nil
}
