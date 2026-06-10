package config

import (
	"fmt"
	"net/netip"
)

const (
	defaultPortHostAddr = "127.0.0.1"
	wildcardHostAddr    = "0.0.0.0"
	minGuestPort        = 1
	minHostPort         = 0
	maxTCPPort          = 65535
)

type portClaim struct {
	hostAddr string
	host     int
	baseHost int
	guest    int
}

func validatePorts(ports []PortForward, replicas int) error {
	var claimed []portClaim
	for _, port := range ports {
		if err := validateGuestPort(port.GuestPort); err != nil {
			return err
		}
		if err := validateHostPort(port.HostPort); err != nil {
			return err
		}
		if err := validatePortAddress("host", port.HostAddr); err != nil {
			return err
		}
		if err := validatePortAddress("guest", port.GuestAddr); err != nil {
			return err
		}
		if port.HostPort > minHostPort {
			if err := validateReplicaHostPortRange(port.HostPort, replicas); err != nil {
				return err
			}
			for r := 0; r < replicas; r++ {
				host := port.HostPort + r
				hostAddr := effectiveHostAddr(port.HostAddr)
				if prev, ok := conflictingPortClaim(claimed, hostAddr, host); ok {
					return fmt.Errorf(
						"host port %s:%d is claimed by both mapping %s:%d:%d and %s:%d:%d at replica %d",
						hostAddr, host, prev.hostAddr, prev.baseHost, prev.guest, hostAddr, port.HostPort, port.GuestPort, r)
				}
				claimed = append(claimed, portClaim{hostAddr: hostAddr, host: host, baseHost: port.HostPort, guest: port.GuestPort})
			}
		}
		if err := validatePortProtocol(port.Protocol); err != nil {
			return err
		}
	}
	return nil
}

func conflictingPortClaim(claimed []portClaim, hostAddr string, hostPort int) (portClaim, bool) {
	for _, prev := range claimed {
		if prev.host == hostPort && (prev.hostAddr == hostAddr || prev.hostAddr == wildcardHostAddr || hostAddr == wildcardHostAddr) {
			return prev, true
		}
	}
	return portClaim{}, false
}

func validateReplicaHostPortRange(hostPort, replicas int) error {
	top := hostPort + replicas - 1
	if top > maxTCPPort {
		return fmt.Errorf(
			"host port %d with replicas %d would overflow to %d (must be <= %d)",
			hostPort, replicas, top, maxTCPPort)
	}
	return nil
}

func validateGuestPort(port int) error {
	if port < minGuestPort || port > maxTCPPort {
		return fmt.Errorf("guest port %d is out of range", port)
	}
	return nil
}

func validateHostPort(port int) error {
	if port < minHostPort || port > maxTCPPort {
		return fmt.Errorf("host port %d is out of range", port)
	}
	return nil
}

func validatePortProtocol(protocol string) error {
	if protocol != DefaultProtocol {
		return fmt.Errorf("protocol %q is unsupported; only %s is implemented", protocol, DefaultProtocol)
	}
	return nil
}

func validatePortAddress(kind, addr string) error {
	if addr == "" {
		return nil
	}
	parsed, err := netip.ParseAddr(addr)
	if err != nil {
		return fmt.Errorf("%s address %q is invalid: %w", kind, addr, err)
	}
	if !parsed.Is4() {
		return fmt.Errorf("%s address %q must be IPv4", kind, addr)
	}
	return nil
}

func effectiveHostAddr(addr string) string {
	if addr == "" {
		return defaultPortHostAddr
	}
	return addr
}
