package console

import "io"

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
		buf = appendCleanCR(buf, len(p) > 0 && p[0] == '\n')
	}

	for i := 0; i < len(p); i++ {
		if p[i] == '\r' {
			if i+1 < len(p) {
				buf = appendCleanCR(buf, p[i+1] == '\n')
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

func appendCleanCR(buf []byte, followedByLF bool) []byte {
	buf = append(buf, '\r')
	if !followedByLF {
		buf = append(buf, eraseEOL...)
	}
	return buf
}
