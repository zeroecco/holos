package console

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"golang.org/x/term"
)

const escapeChar = 0x1d // Ctrl-]

// Attach connects to a QEMU serial console unix socket and proxies
// stdin/stdout in raw terminal mode. The session ends when the user
// presses Ctrl-] or the socket closes.
func Attach(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("connect to serial console: %w", err)
	}
	defer conn.Close()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("set terminal raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Fprintf(os.Stdout, "Connected to serial console. Press Ctrl-] to detach.\r\n")

	var wg sync.WaitGroup
	done := make(chan struct{})

	// socket -> stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(os.Stdout, conn)
		close(done)
	}()

	// stdin -> socket (with escape detection)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 256)
		for {
			select {
			case <-done:
				return
			default:
			}

			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				if buf[i] == escapeChar {
					return
				}
			}
			conn.Write(buf[:n])
		}
	}()

	wg.Wait()
	return nil
}
