//go:build integration

package integration

import (
	"net"
	"testing"
)

// reserveLocalPort opens a TCP listener on 127.0.0.1, registers a cleanup
// to close it, and returns the port number. Any attempt to bind the same
// port from a different socket will fail until the test ends.
func reserveLocalPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type")
	}
	return addr.Port
}
