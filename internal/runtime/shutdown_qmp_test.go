package runtime

import (
	"bufio"
	"net"
	"path/filepath"
	"testing"
)

func TestRequestPowerdown(t *testing.T) {
	t.Parallel()

	socketPath := filepath.Join(t.TempDir(), instanceQMPSocketFilename)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen qmp: %v", err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		if _, err := conn.Write([]byte(`{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}` + "\n")); err != nil {
			return
		}
		reader := bufio.NewReader(conn)
		for i := 0; i < 2; i++ {
			if _, err := reader.ReadBytes('\n'); err != nil {
				return
			}
			if _, err := conn.Write([]byte(`{"return":{}}` + "\n")); err != nil {
				return
			}
		}
	}()

	if !requestPowerdown(socketPath) {
		t.Fatal("requestPowerdown returned false for ACKing qmp server")
	}
	<-done
}

func TestRequestPowerdownMissingSocket(t *testing.T) {
	t.Parallel()

	if requestPowerdown(filepath.Join(t.TempDir(), "missing.sock")) {
		t.Fatal("requestPowerdown returned true for missing socket")
	}
}
