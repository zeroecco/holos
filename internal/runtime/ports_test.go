package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

func TestAllocatePortsCarriesBindAddresses(t *testing.T) {
	t.Setenv("HOLOS_TEST_EPHEMERAL_PORTS", "25000")

	ports, err := allocatePorts(config.Manifest{
		Ports: []config.PortForward{
			{Name: "http", HostAddr: "0.0.0.0", HostPort: 8080, GuestAddr: "10.0.2.15", GuestPort: 80, Protocol: "tcp"},
			{Name: "admin", HostAddr: "127.0.0.2", GuestPort: 9000, Protocol: "tcp"},
		},
	}, 2)
	if err != nil {
		t.Fatalf("allocatePorts: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("allocatePorts returned %d ports, want 2", len(ports))
	}
	if ports[0].HostAddr != "0.0.0.0" || ports[0].HostPort != 8082 || ports[0].GuestAddr != "10.0.2.15" || ports[0].GuestPort != 80 {
		t.Fatalf("static port = %+v", ports[0])
	}
	if ports[1].HostAddr != "127.0.0.2" || ports[1].HostPort != 25000 || ports[1].GuestPort != 9000 {
		t.Fatalf("ephemeral port = %+v", ports[1])
	}
}
