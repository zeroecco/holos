package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

const testEphemeralPortsEnv = "HOLOS_TEST_EPHEMERAL_PORTS"

var (
	testEphemeralPortsMu    sync.Mutex
	testEphemeralPortsValue string
	testEphemeralPortsIndex int
)

func nextTestEphemeralTCPPort() (int, bool, error) {
	raw := os.Getenv(testEphemeralPortsEnv)
	if raw == "" {
		return 0, false, nil
	}

	testEphemeralPortsMu.Lock()
	defer testEphemeralPortsMu.Unlock()

	if raw != testEphemeralPortsValue {
		testEphemeralPortsValue = raw
		testEphemeralPortsIndex = 0
	}

	ports := parseTestEphemeralPorts(raw)
	if testEphemeralPortsIndex >= len(ports) {
		return 0, true, fmt.Errorf("%s exhausted after %d allocations", testEphemeralPortsEnv, len(ports))
	}
	value := ports[testEphemeralPortsIndex]
	testEphemeralPortsIndex++

	port, err := parseTestEphemeralPort(value)
	if err != nil {
		return 0, true, fmt.Errorf("invalid %s entry %q", testEphemeralPortsEnv, value)
	}
	return port, true, nil
}

func parseTestEphemeralPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}

func parseTestEphemeralPorts(raw string) []string {
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
