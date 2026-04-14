package console

import (
	"fmt"
	"io"
	"net"
	"os"

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

	// Send a carriage return to nudge any login prompt into redisplaying.
	conn.Write([]byte("\r"))

	// Either goroutine finishing means the session is over.
	errc := make(chan error, 2)

	// socket -> stdout
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		errc <- err
	}()

	// stdin -> socket (with escape detection)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				errc <- err
				return
			}
			for i := 0; i < n; i++ {
				if buf[i] == escapeChar {
					errc <- nil
					return
				}
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				errc <- err
				return
			}
		}
	}()

	// Wait for the first goroutine to finish, then close the conn
	// to unblock the other.
	<-errc
	conn.Close()

	return nil
}
