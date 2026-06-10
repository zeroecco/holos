package runtime

import (
	"fmt"
	"os"

	"github.com/zeroecco/holos/internal/qmp"
)

const qmpDebugEnv = "HOLOS_DEBUG_QMP"

// requestPowerdown dials the QMP socket, completes the handshake, and
// sends system_powerdown. It returns true only when the server ACKs the
// command. Any failure (missing socket, handshake timeout, QMP error) is
// swallowed and reported as false so the caller falls through to SIGTERM.
// If the HOLOS_DEBUG_QMP environment variable is set, failures are logged
// to stderr to aid debugging.
func requestPowerdown(socketPath string) bool {
	debug := os.Getenv(qmpDebugEnv) != ""
	client, err := qmp.Dial(socketPath, qmpHandshakeTimeout)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "qmp dial %s: %v\n", socketPath, err)
		}
		return false
	}
	defer client.Close()
	if err := client.Powerdown(qmpHandshakeTimeout); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "qmp powerdown %s: %v\n", socketPath, err)
		}
		return false
	}
	return true
}
