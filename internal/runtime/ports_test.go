package runtime

import (
	"net"
	"slices"
	"strconv"
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

func TestAllocatePortsCarriesBindAddresses(t *testing.T) {
	t.Setenv(testEphemeralPortsEnv, "25000")

	ports, err := allocatePorts(config.Manifest{
		Ports: []config.PortForward{
			{Name: "http", HostAddr: "0.0.0.0", HostPort: 8080, GuestAddr: "10.0.2.15", GuestPort: 80, Protocol: config.DefaultProtocol},
			{Name: "admin", HostAddr: "127.0.0.2", GuestPort: 9000, Protocol: config.DefaultProtocol},
		},
	}, 2)
	if err != nil {
		t.Fatalf("allocatePorts: %v", err)
	}
	want := []qemu.PortMapping{
		{Name: "http", HostAddr: "0.0.0.0", HostPort: 8082, GuestAddr: "10.0.2.15", GuestPort: 80, Protocol: config.DefaultProtocol},
		{Name: "admin", HostAddr: "127.0.0.2", HostPort: 25000, GuestPort: 9000, Protocol: config.DefaultProtocol},
	}
	if !slices.Equal(ports, want) {
		t.Fatalf("allocatePorts = %+v, want %+v", ports, want)
	}
}

func TestEnsureTCPPortAvailableReportsOccupiedPort(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen(tcpNetwork, net.JoinHostPort(defaultHostAddr, ephemeralPortSpec))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	err = ensureTCPPortAvailable(defaultHostAddr, addr.Port)
	if err == nil {
		t.Fatal("ensureTCPPortAvailable succeeded for occupied port")
	}
	assertErrorContains(t, err, "host port 127.0.0.1:"+strconv.Itoa(addr.Port)+" is unavailable")
}

func TestEffectiveHostAddrDefaultsToLoopback(t *testing.T) {
	t.Parallel()

	if got := effectiveHostAddr(""); got != defaultHostAddr {
		t.Fatalf("effectiveHostAddr(\"\") = %q, want %q", got, defaultHostAddr)
	}
	if got := effectiveHostAddr("0.0.0.0"); got != "0.0.0.0" {
		t.Fatalf("effectiveHostAddr keeps explicit address = %q", got)
	}
}
