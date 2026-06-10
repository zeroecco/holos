package images

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// bodyIdleTimeout bounds how long a download may go without receiving
// any bytes. The Transport only covers connect/TLS/header phases, so
// once headers arrive a silent mirror can keep the TCP stream open
// indefinitely while `holos pull` hangs.
var bodyIdleTimeout = 60 * time.Second

// idleTimeoutReader wraps an HTTP response body with a watchdog that
// fires if no bytes arrive within `timeout`. When it fires it calls
// the request's cancel func, which aborts the outstanding Transport
// Read and makes io.Copy return quickly.
type idleTimeoutReader struct {
	r       io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	fired   atomicBool
}

// newIdleTimeoutReader starts the watchdog immediately so that a
// mirror which never sends the first byte is still caught.
func newIdleTimeoutReader(r io.ReadCloser, timeout time.Duration, cancel context.CancelFunc) *idleTimeoutReader {
	itr := &idleTimeoutReader{r: r, timeout: timeout}
	itr.timer = time.AfterFunc(timeout, func() {
		itr.fired.Store(true)
		cancel()
	})
	return itr
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		// Reset keeps the watchdog honest on fast connections; races
		// with an in-flight expiry are acceptable.
		i.timer.Reset(i.timeout)
	}
	return n, err
}

// Stop prevents the watchdog from firing after a normal end-of-body.
func (i *idleTimeoutReader) Stop() {
	i.timer.Stop()
}

func (i *idleTimeoutReader) TimedOut() bool {
	return i.fired.Load()
}

// atomicBool is a tiny wrapper so the watchdog's "did I fire?" flag
// is safe to read from the Read goroutine while the timer goroutine
// may be writing it.
type atomicBool struct{ v atomic.Bool }

func (b *atomicBool) Store(x bool) { b.v.Store(x) }
func (b *atomicBool) Load() bool   { return b.v.Load() }
