package qemu

import (
	"fmt"
	"strings"
)

const (
	userNetdevID      = "net0"
	socketNetdevID    = "net1"
	userNetdevBase    = "user,id=" + userNetdevID
	bridgeBackend     = "bridge"
	defaultHostAddr   = "127.0.0.1"
	tcpProtocol       = "tcp"
	udpProtocol       = "udp"
	sshGuestPort      = 22
	hostForwardFormat = "hostfwd=%s:%s:%d-%s"
	netdevKey         = "netdev"
	macKey            = "mac"
)

func buildNetdev(ports []PortMapping, sshPort int) (string, error) {
	options := []string{userNetdevBase}
	for _, port := range ports {
		forward, err := portHostForward(port)
		if err != nil {
			return "", err
		}
		options = append(options, forward)
	}
	if sshPort > 0 {
		options = append(options, hostForward(tcpProtocol, defaultHostAddr, sshPort, "", sshGuestPort))
	}
	return strings.Join(options, ","), nil
}

func portHostForward(port PortMapping) (string, error) {
	if port.Protocol != tcpProtocol && port.Protocol != udpProtocol {
		return "", fmt.Errorf("unsupported port mapping protocol %q", port.Protocol)
	}
	hostAddr := hostForwardHostAddr(port.HostAddr)
	return hostForward(port.Protocol, hostAddr, port.HostPort, port.GuestAddr, port.GuestPort), nil
}

func hostForwardHostAddr(hostAddr string) string {
	if hostAddr == "" {
		return defaultHostAddr
	}
	return hostAddr
}

func hostForward(protocol, hostAddr string, hostPort int, guestAddr string, guestPort int) string {
	return fmt.Sprintf(hostForwardFormat, protocol, hostAddr, hostPort, hostForwardGuestTarget(guestAddr, guestPort))
}

func hostForwardGuestTarget(guestAddr string, guestPort int) string {
	if guestAddr != "" {
		return fmt.Sprintf("%s:%d", guestAddr, guestPort)
	}
	return fmt.Sprintf(":%d", guestPort)
}
