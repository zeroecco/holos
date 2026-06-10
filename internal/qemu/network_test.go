package qemu

import "testing"

func TestBuildNetdev(t *testing.T) {
	t.Parallel()

	netdev, err := buildNetdev([]PortMapping{
		{HostPort: 8080, GuestPort: 80, Protocol: tcpProtocol},
		{HostPort: 5353, GuestPort: 5353, Protocol: udpProtocol},
		{HostAddr: "0.0.0.0", HostPort: 9000, GuestAddr: "10.0.2.15", GuestPort: 9000, Protocol: tcpProtocol},
	}, 2022)
	if err != nil {
		t.Fatalf("buildNetdev: %v", err)
	}

	want := "user,id=net0" +
		",hostfwd=tcp:127.0.0.1:8080-:80" +
		",hostfwd=udp:127.0.0.1:5353-:5353" +
		",hostfwd=tcp:0.0.0.0:9000-10.0.2.15:9000" +
		",hostfwd=tcp:127.0.0.1:2022-:22"
	if netdev != want {
		t.Fatalf("buildNetdev = %q, want %q", netdev, want)
	}
}

func TestBuildNetdevRejectsUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	if _, err := buildNetdev([]PortMapping{{HostPort: 5353, GuestPort: 5353, Protocol: "sctp"}}, 0); err == nil {
		t.Fatal("buildNetdev sctp error = nil, want error")
	}
}
