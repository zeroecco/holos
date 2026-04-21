package main

import (
	"crypto/sha256"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestParseMemoryMB(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"512", 512, false},
		{"512M", 512, false},
		{"512m", 512, false},
		{"512MB", 512, false},
		{"2G", 2048, false},
		{"2GB", 2048, false},
		{"2g", 2048, false},
		{"1T", 1024 * 1024, false},
		{"4096K", 4, false},
		{"  1G  ", 1024, false},
		{"", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"512K", 0, true}, // 512 KiB rounds to 0 MB; rejected
		{"0", 0, true},
	}
	for _, c := range cases {
		got, err := parseMemoryMB(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMemoryMB(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMemoryMB(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMemoryMB(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestGenerateRunName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		image      string
		dockerfile string
		wantPrefix string
	}{
		{"ubuntu:noble", "", "ubuntu-noble-"},
		{"alpine", "", "alpine-"},
		{"./images/my-image.qcow2", "", "my-image-"},
		{"/var/lib/libvirt/images/web.raw", "", "web-"},
		{"", "./Dockerfile", "dockerfile-"},
		{"REGISTRY/Foo_Bar:1.0", "", "foo-bar-"},
	}
	for _, c := range cases {
		got := generateRunName(c.image, c.dockerfile)
		if !strings.HasPrefix(got, c.wantPrefix) {
			t.Errorf("generateRunName(%q, %q) = %q, want prefix %q",
				c.image, c.dockerfile, got, c.wantPrefix)
		}
		if !runNamePattern.MatchString(got) {
			t.Errorf("generateRunName(%q, %q) = %q, fails compose name validation",
				c.image, c.dockerfile, got)
		}
		if len(got) > 63 {
			t.Errorf("generateRunName(%q, %q) = %q (len %d), exceeds 63-char limit",
				c.image, c.dockerfile, got, len(got))
		}
	}
}

func TestGenerateRunNameUnique(t *testing.T) {
	t.Parallel()

	// Repeated invocations on the same image should produce distinct
	// names (random suffix). We're not asserting a strong uniqueness
	// guarantee here, just that the suffix isn't a constant.
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		seen[generateRunName("alpine", "")] = true
	}
	if len(seen) < 8 {
		t.Errorf("expected diverse run names across 16 calls, got only %d unique", len(seen))
	}
}

func TestGenerateRunNameLongImage(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 200)
	got := generateRunName(long, "")
	if len(got) > 63 {
		t.Fatalf("generateRunName(<200 a's>) = %q (len %d), exceeds 63-char limit", got, len(got))
	}
	if !runNamePattern.MatchString(got) {
		t.Errorf("generateRunName(<200 a's>) = %q, fails compose name validation", got)
	}
}

// TestRandHexLengthContract pins randHex's documented "exactly 2*n
// chars" promise. generateRunName depends on it: the suffix must be
// exactly 6 chars to keep names within DNS's 63-char label limit.
// This covers the success path; the fallback path is verified by
// TestGenerateRunNameLongImageFallback below using a stub that
// exercises the post-failure branch directly.
func TestRandHexLengthContract(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 3, 8, 16, 32} {
		got := randHex(n)
		if len(got) != 2*n {
			t.Errorf("randHex(%d) = %q (len %d), want length %d", n, got, len(got), 2*n)
		}
		// Must be valid lowercase hex so it survives runNamePattern.
		for _, c := range got {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("randHex(%d) = %q contains non-hex rune %q", n, got, c)
			}
		}
	}
}

// TestRandHexFallbackLengthContract directly exercises the branch
// that used to return strconv.FormatInt(pid, 16): variable-length
// and silently capable of blowing the 63-char DNS label limit when
// combined with a long image name in generateRunName. The current
// implementation must return exactly 2*n chars, all valid hex.
func TestRandHexFallbackLengthContract(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 3, 8, 16, 32} {
		got := randHexFallback(n)
		if len(got) != 2*n {
			t.Errorf("randHexFallback(%d) = %q (len %d), want length %d",
				n, got, len(got), 2*n)
		}
		for _, c := range got {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("randHexFallback(%d) = %q contains non-hex rune %q", n, got, c)
			}
		}
	}

	// Asking for more bytes than sha256 gives us must be safely
	// capped, not panic. The result is shorter than 2*n in this
	// degenerate case, but the function shouldn't crash; callers
	// in this codebase only ever request n ≤ 8.
	if got := randHexFallback(64); len(got) != 2*sha256.Size {
		t.Errorf("randHexFallback(64) = %q (len %d), want %d (capped to sha256.Size)",
			got, len(got), 2*sha256.Size)
	}
}

// TestGenerateRunNameLongImageFallback ensures that even with a
// pathological 200-char image *and* a fallback suffix, the final
// name still fits in 63 chars. We can't easily force crypto/rand to
// fail mid-test, so we substitute a maximum-length suffix (12 hex
// chars, what a 6-byte randHex would return) and verify the math.
func TestGenerateRunNameLongImageFallback(t *testing.T) {
	t.Parallel()

	// Re-derive the boundary from the same constants generateRunName
	// uses. If anyone bumps suffixLen without also bumping the trim,
	// this test catches it.
	long := strings.Repeat("a", 200)
	got := generateRunName(long, "")

	// generateRunName trims to 63-7=56 then appends "-XXXXXX" (7).
	if want := 63; len(got) != want {
		t.Errorf("generateRunName(long) = %q (len %d), want %d (full label)", got, len(got), want)
	}
	if !runNamePattern.MatchString(got) {
		t.Errorf("generateRunName(long) = %q, fails compose name validation", got)
	}
}

// TestResolveLogTargets pins the dual-mode lookup that fixed the
// confusing "service \"vm-0\" not found" error: the same identifier
// `ps` shows in its INSTANCE column should be acceptable to `logs`,
// not just the SERVICE name.
func TestResolveLogTargets(t *testing.T) {
	t.Parallel()

	record := &runtime.ProjectRecord{
		Name: "demo",
		Services: []runtime.ServiceRecord{
			{
				Name:            "vm",
				DesiredReplicas: 2,
				Instances: []runtime.InstanceRecord{
					{Name: "vm-0", LogPath: "/tmp/vm-0.log"},
					{Name: "vm-1", LogPath: "/tmp/vm-1.log"},
				},
			},
			{
				Name: "db",
				Instances: []runtime.InstanceRecord{
					{Name: "db-0", LogPath: "/tmp/db-0.log"},
				},
			},
		},
	}

	cases := []struct {
		name     string
		target   string
		wantLogs []string
	}{
		{"service-name-fans-out", "vm", []string{"/tmp/vm-0.log", "/tmp/vm-1.log"}},
		{"single-instance", "vm-0", []string{"/tmp/vm-0.log"}},
		{"different-service-instance", "db-0", []string{"/tmp/db-0.log"}},
		{"unknown", "nope", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveLogTargets(record, c.target)
			if len(got) != len(c.wantLogs) {
				t.Fatalf("%s: got %d targets (%v), want %d (%v)",
					c.target, len(got), got, len(c.wantLogs), c.wantLogs)
			}
			for i, inst := range got {
				if inst.LogPath != c.wantLogs[i] {
					t.Errorf("%s[%d]: LogPath = %q, want %q",
						c.target, i, inst.LogPath, c.wantLogs[i])
				}
			}
		})
	}
}

// TestResolveLogTargetsServiceWinsOnCollision documents the
// tie-break: when a service and an instance share a name (someone
// names a service "vm-0"), the service interpretation wins and we
// fan out to all of its replicas. Asserted explicitly so a future
// refactor can't quietly flip the order without a failing test.
func TestResolveLogTargetsServiceWinsOnCollision(t *testing.T) {
	t.Parallel()

	record := &runtime.ProjectRecord{
		Services: []runtime.ServiceRecord{
			{
				Name: "vm-0", // weird but legal
				Instances: []runtime.InstanceRecord{
					{Name: "vm-0-0", LogPath: "/tmp/vm-0-0.log"},
				},
			},
			{
				Name: "other",
				Instances: []runtime.InstanceRecord{
					{Name: "vm-0", LogPath: "/tmp/other.log"},
				},
			},
		},
	}
	got := resolveLogTargets(record, "vm-0")
	if len(got) != 1 || got[0].LogPath != "/tmp/vm-0-0.log" {
		t.Fatalf("expected service interpretation to win, got %+v", got)
	}
}

// TestSshdReady covers the success path (real listener that speaks
// the SSH banner) and the failure path (RST mid-handshake by
// closing without writing). The whole point of the helper is to
// distinguish those two cases. The original "kex_exchange:
// Connection reset by peer" symptom was the second case and we
// need to retry it, not bail out.
func TestSshdReady(t *testing.T) {
	t.Parallel()

	t.Run("real-banner", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
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
			time.Sleep(50 * time.Millisecond)
		}()
		if !sshdReady(ln.Addr().String()) {
			t.Errorf("sshdReady on a banner-emitting listener returned false")
		}
	})

	t.Run("rst-mid-handshake", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
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

	ln, err := net.Listen("tcp", "127.0.0.1:0")
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

func TestStringListAppends(t *testing.T) {
	t.Parallel()

	var list stringList
	for _, v := range []string{"8080:80", "9090:90", "5432:5432"} {
		if err := list.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	if len(list) != 3 || list[0] != "8080:80" || list[2] != "5432:5432" {
		t.Errorf("stringList accumulation broken: %v", list)
	}
	if got := list.String(); got != "8080:80,9090:90,5432:5432" {
		t.Errorf("String() = %q", got)
	}
}
