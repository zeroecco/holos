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

	conn.Write([]byte("\r"))

	errc := make(chan error, 2)

	// socket -> stdout (with \r cleanup)
	go func() {
		_, err := io.Copy(&crCleanWriter{w: os.Stdout}, conn)
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

	<-errc
	conn.Close()

	return nil
}

// crCleanWriter wraps a writer and inserts an ANSI "erase to end of
// line" escape after every bare \r (i.e. \r not followed by \n).
// This prevents APT-style progress bars from leaving line remnants
// when overwritten by shorter text.
//
// A \r at a chunk boundary is deferred until the next Write so we can
// check whether the following byte is \n before deciding to erase.
type crCleanWriter struct {
	w      io.Writer
	pendCR bool
}

var eraseEOL = []byte("\x1b[K")

func (c *crCleanWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(p))

	if c.pendCR {
		c.pendCR = false
		if len(p) > 0 && p[0] == '\n' {
			buf = append(buf, '\r')
		} else {
			buf = append(buf, '\r')
			buf = append(buf, eraseEOL...)
		}
	}

	for i := 0; i < len(p); i++ {
		if p[i] == '\r' {
			if i+1 < len(p) {
				buf = append(buf, '\r')
				if p[i+1] != '\n' {
					buf = append(buf, eraseEOL...)
				}
			} else {
				c.pendCR = true
			}
		} else {
			buf = append(buf, p[i])
		}
	}

	if len(buf) > 0 {
		if _, err := c.w.Write(buf); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
