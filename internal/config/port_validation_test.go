package config

import "testing"

func TestValidateGuestPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{name: "below minimum", port: minGuestPort - 1, wantErr: true},
		{name: "minimum", port: minGuestPort},
		{name: "maximum", port: maxTCPPort},
		{name: "above maximum", port: maxTCPPort + 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateGuestPort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateGuestPort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{name: "below minimum", port: minHostPort - 1, wantErr: true},
		{name: "ephemeral", port: minHostPort},
		{name: "maximum", port: maxTCPPort},
		{name: "above maximum", port: maxTCPPort + 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateHostPort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHostPort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePortsSkipsReplicaClaimsForEphemeralHostPorts(t *testing.T) {
	t.Parallel()

	ports := []PortForward{
		{HostPort: minHostPort, GuestPort: 80, Protocol: DefaultProtocol},
		{HostPort: minHostPort, GuestPort: 81, Protocol: DefaultProtocol},
	}
	if err := validatePorts(ports, maxTCPPort+1); err != nil {
		t.Fatalf("validatePorts with ephemeral host ports: %v", err)
	}
}

func TestValidatePortProtocol(t *testing.T) {
	t.Parallel()

	if err := validatePortProtocol(DefaultProtocol); err != nil {
		t.Fatalf("validatePortProtocol(%q): %v", DefaultProtocol, err)
	}
	if err := validatePortProtocol(ProtocolUDP); err != nil {
		t.Fatalf("validatePortProtocol(%q): %v", ProtocolUDP, err)
	}
	if err := validatePortProtocol("sctp"); err == nil {
		t.Fatal("validatePortProtocol(sctp) succeeded, want error")
	}
}

func TestEffectiveHostAddr(t *testing.T) {
	t.Parallel()

	if got := effectiveHostAddr(""); got != defaultPortHostAddr {
		t.Fatalf("effectiveHostAddr(empty) = %q, want %q", got, defaultPortHostAddr)
	}
	if got := effectiveHostAddr("127.0.0.2"); got != "127.0.0.2" {
		t.Fatalf("effectiveHostAddr(explicit) = %q", got)
	}
}

func TestConflictingPortClaim(t *testing.T) {
	t.Parallel()

	claims := []portClaim{
		{hostAddr: defaultPortHostAddr, protocol: ProtocolTCP, host: 8080, baseHost: 8080, guest: 80},
		{hostAddr: "127.0.0.2", protocol: ProtocolTCP, host: 8081, baseHost: 8081, guest: 81},
	}

	got, ok := conflictingPortClaim(claims, defaultPortHostAddr, ProtocolTCP, 8080)
	if !ok {
		t.Fatal("conflictingPortClaim matching address/port ok = false, want true")
	}
	if got.guest != 80 {
		t.Fatalf("conflictingPortClaim guest = %d, want 80", got.guest)
	}

	if _, ok := conflictingPortClaim(claims, defaultPortHostAddr, ProtocolTCP, 8082); ok {
		t.Fatal("conflictingPortClaim different port ok = true, want false")
	}
	if _, ok := conflictingPortClaim(claims, "127.0.0.3", ProtocolTCP, 8081); ok {
		t.Fatal("conflictingPortClaim different address ok = true, want false")
	}
	if _, ok := conflictingPortClaim(claims, defaultPortHostAddr, ProtocolUDP, 8080); ok {
		t.Fatal("conflictingPortClaim different protocol ok = true, want false")
	}
	if _, ok := conflictingPortClaim(claims, wildcardHostAddr, ProtocolTCP, 8081); !ok {
		t.Fatal("conflictingPortClaim wildcard address ok = false, want true")
	}
}

func TestValidateReplicaHostPortRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostPort int
		replicas int
		wantErr  bool
	}{
		{name: "single maximum", hostPort: maxTCPPort, replicas: 1},
		{name: "range reaches maximum", hostPort: maxTCPPort - 2, replicas: 3},
		{name: "range overflows maximum", hostPort: maxTCPPort - 1, replicas: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateReplicaHostPortRange(tt.hostPort, tt.replicas)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateReplicaHostPortRange(%d, %d) error = %v, wantErr %v", tt.hostPort, tt.replicas, err, tt.wantErr)
			}
		})
	}
}
