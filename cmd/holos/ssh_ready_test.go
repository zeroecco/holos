package main

import (
	"errors"
	"net"
	"testing"
	"time"
)

// TestSshdReady covers the success path (real listener that speaks
// the SSH banner) and the failure path (RST mid-handshake by
// closing without writing). The whole point of the helper is to
// distinguish those two cases. The original "kex_exchange:
// Connection reset by peer" symptom was the second case and we
// need to retry it, not bail out.
func TestSshdReady(t *testing.T) {
	t.Parallel()

	t.Run("real-banner", func(t *testing.T) {
		ln, err := net.Listen(sshdProbeNetwork, "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.6\r\n"))
			time.Sleep(sshdReadyInitialBackoff / 4)
		}()
		if !sshdReady(ln.Addr().String()) {
			t.Errorf("sshdReady on a banner-emitting listener returned false")
		}
	})

	t.Run("rst-mid-handshake", func(t *testing.T) {
		ln, err := net.Listen(sshdProbeNetwork, "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Close immediately, no banner. Mimics sshd
			// bouncing during host-key regen.
			_ = c.Close()
		}()
		if sshdReady(ln.Addr().String()) {
			t.Errorf("sshdReady on a connection-resetting listener returned true")
		}
	})

	t.Run("non-ssh-banner", func(t *testing.T) {
		ln, err := net.Listen(sshdProbeNetwork, "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\n"))
		}()
		if sshdReady(ln.Addr().String()) {
			t.Errorf("sshdReady on a non-SSH banner returned true")
		}
	})

	t.Run("nothing-listening", func(t *testing.T) {
		// 127.0.0.1:1 is reliably closed; SLIRP would surface
		// this same way for a not-yet-bound guest port.
		if sshdReady("127.0.0.1:1") {
			t.Errorf("sshdReady against a closed port returned true")
		}
	})
}

// TestWaitForSSHReadyEventuallySucceeds proves the polling loop
// recovers when a listener that's initially silent starts emitting
// the SSH banner mid-wait. Mirrors the real cloud-init scenario
// where sshd comes up after a short delay.
func TestWaitForSSHReadyEventuallySucceeds(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen(sshdProbeNetwork, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ready := make(chan struct{})
	go func() {
		// First two connections: drop without writing to mimic
		// sshd bouncing. Third onward: emit a real banner.
		drops := 0
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if drops < 2 {
				drops++
				_ = c.Close()
				continue
			}
			_, _ = c.Write([]byte("SSH-2.0-test\r\n"))
			_ = c.Close()
			select {
			case <-ready:
			default:
				close(ready)
			}
		}
	}()

	if err := waitForSSHReady(ln.Addr().String(), 5*time.Second); err != nil {
		t.Fatalf("waitForSSHReady: %v", err)
	}
}

func TestSSHReadyProbeConstants(t *testing.T) {
	t.Parallel()

	if sshdBannerProbeBytes != len(sshdBannerPrefix) {
		t.Fatalf("sshdBannerProbeBytes = %d, want %d", sshdBannerProbeBytes, len(sshdBannerPrefix))
	}
	if sshdReadyInitialBackoff <= 0 || sshdReadyInitialBackoff >= sshdReadyMaxBackoff {
		t.Fatalf("invalid sshd ready backoff bounds: initial=%s max=%s", sshdReadyInitialBackoff, sshdReadyMaxBackoff)
	}
}

func TestSSHDBannerReadComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		err  error
		want bool
	}{
		{name: "complete", n: sshdBannerProbeBytes, want: true},
		{name: "longer than probe", n: sshdBannerProbeBytes + 1, want: true},
		{name: "short", n: sshdBannerProbeBytes - 1},
		{name: "error", n: sshdBannerProbeBytes, err: errors.New("read failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sshdBannerReadComplete(tt.n, tt.err); got != tt.want {
				t.Fatalf("sshdBannerReadComplete = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNextSSHReadyBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		delay time.Duration
		want  time.Duration
	}{
		{name: "doubles below max", delay: sshdReadyInitialBackoff, want: sshdReadyInitialBackoff * 2},
		{name: "clamps above max", delay: sshdReadyMaxBackoff - time.Millisecond, want: sshdReadyMaxBackoff},
		{name: "stays at max", delay: sshdReadyMaxBackoff, want: sshdReadyMaxBackoff},
		{name: "stays above max", delay: sshdReadyMaxBackoff + time.Second, want: sshdReadyMaxBackoff + time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextSSHReadyBackoff(tt.delay); got != tt.want {
				t.Fatalf("nextSSHReadyBackoff(%s) = %s, want %s", tt.delay, got, tt.want)
			}
		})
	}
}
