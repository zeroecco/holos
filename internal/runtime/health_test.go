package runtime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// TestProbeHealthcheck_Success stands up a real in-process ssh server
// that accepts the project key, answers a single exec request with exit
// code 0, and proves the runtime probe reports the guest as healthy.
func TestProbeHealthcheck_Success(t *testing.T) {
	t.Parallel()

	addr, keyPath, stop := startFakeSSHServer(t, 0)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := probeHealthcheck(ctx, addr, "tester", keyPath,
		[]string{"/bin/true"}, 2*time.Second); err != nil {
		t.Fatalf("expected healthy, got: %v", err)
	}
}

// TestProbeHealthcheck_NonZeroExit confirms we distinguish non-zero
// exit status from transport errors. The error message includes the
// observed exit code so `holos up` can surface actionable details.
func TestProbeHealthcheck_NonZeroExit(t *testing.T) {
	t.Parallel()

	addr, keyPath, stop := startFakeSSHServer(t, 2)
	defer stop()

	err := probeHealthcheck(context.Background(), addr, "tester", keyPath,
		[]string{"/bin/false"}, 2*time.Second)
	assertErrorContains(t, err, "exit=2")
}

// TestProbeHealthcheck_DialFailure ensures a dead port surfaces as a
// dial error rather than a panic or hang.
func TestProbeHealthcheck_DialFailure(t *testing.T) {
	t.Parallel()

	keyPath := writeTempPrivateKey(t)

	// Port 1 is never bound on macOS/Linux; connect refuses quickly.
	err := probeHealthcheck(context.Background(), "127.0.0.1:1", "tester", keyPath,
		[]string{"true"}, 500*time.Millisecond)
	assertErrorContains(t, err, "dial")
}

// TestProbeHealthcheck_Bypass verifies that setting HOLOS_HEALTH_BYPASS
// short-circuits the probe so dev/test runs without a real VM pass
// through ordering checks without any ssh traffic.
func TestProbeHealthcheck_Bypass(t *testing.T) {
	t.Setenv(probeBypassEnv, "1")
	// Use an obviously unreachable address; the bypass must kick in
	// before we attempt to dial.
	if err := probeHealthcheck(context.Background(), "203.0.113.1:22", "nobody", "/does/not/exist",
		[]string{"true"}, time.Second); err != nil {
		t.Fatalf("bypass should return nil; got %v", err)
	}
}

func TestProbeHealthcheckRejectsEmptyCommand(t *testing.T) {
	t.Parallel()

	err := probeHealthcheck(context.Background(), "127.0.0.1:1", "tester", "/does/not/exist", nil, time.Second)
	assertErrorContains(t, err, "empty healthcheck command")
}

func TestShellJoinQuotesArgv(t *testing.T) {
	t.Parallel()

	got := shellJoin([]string{"test", "hello world", "it's", "$HOME"})
	want := "'test' 'hello world' 'it'\\''s' '$HOME'"
	if got != want {
		t.Fatalf("shellJoin() = %q, want %q", got, want)
	}
}

func TestShellQuoteArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "plain", arg: "test", want: "'test'"},
		{name: "empty", arg: "", want: "''"},
		{name: "single quote", arg: "it's", want: "'it'\\''s'"},
		{name: "shell variable", arg: "$HOME", want: "'$HOME'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shellQuoteArg(tt.arg); got != tt.want {
				t.Fatalf("shellQuoteArg(%q) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}
}

// --- fake ssh server helpers ---

// startFakeSSHServer listens on 127.0.0.1:<ephemeral>, accepts a single
// public key auth, and answers every exec request with the given exit
// status. Returns the listen address, the path to the client private
// key, and a cleanup function.
func startFakeSSHServer(t *testing.T, exitStatus uint32) (string, string, func()) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	clientKeyPath := writeClientKey(t, priv)

	serverPub, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	_ = serverPub
	serverSigner, err := ssh.NewSignerFromKey(serverPriv)
	if err != nil {
		t.Fatalf("server signer: %v", err)
	}

	authorizedKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("authorized key: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if string(key.Marshal()) != string(authorizedKey.Marshal()) {
				return nil, fmt.Errorf("unauthorized")
			}
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(serverSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := listener.Addr().String()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFakeSSHConn(conn, cfg, exitStatus)
		}
	}()

	return addr, clientKeyPath, func() {
		_ = listener.Close()
		<-done
	}
}

func handleFakeSSHConn(conn net.Conn, cfg *ssh.ServerConfig, exitStatus uint32) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for ch := range chans {
		if ch.ChannelType() != "session" {
			_ = ch.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		channel, sessionReqs, err := ch.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for req := range sessionReqs {
				switch req.Type {
				case "exec":
					_ = req.Reply(true, nil)
					io.WriteString(channel, "ok\n")
					status := struct{ Status uint32 }{exitStatus}
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(&status))
					return
				default:
					_ = req.Reply(false, nil)
				}
			}
		}()
	}
}

// writeClientKey returns the path to an OpenSSH-format private key on
// disk; the loader in health.go uses ssh.ParsePrivateKey which expects
// the standard OPENSSH wrapper format.
func writeClientKey(t *testing.T, priv ed25519.PrivateKey) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(priv, "healthcheck-test")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

// TestWaitForHealthy_StartPeriodDoesNotConsumeRetries pins the
// two-phase contract: if start_period is long and retries is small,
// failures that happen inside the grace window must not burn retry
// budget. Under the old max(start_period, retries*interval) logic a
// run with start_period=60s, interval=10ms (scaled), retries=3 would
// exhaust the deadline before phase 2 ran at all.
func TestWaitForHealthy_StartPeriodDoesNotConsumeRetries(t *testing.T) {
	t.Parallel()

	var calls int
	// Fail for the entire grace window, succeed on the very first
	// post-grace attempt. If the loop wrongly uses a single combined
	// deadline it will never reach the success path.
	probe := func(ctx context.Context, timeout time.Duration) error {
		calls++
		if calls <= 5 {
			return fmt.Errorf("grace failure #%d", calls)
		}
		return nil
	}

	// Short grace to keep the test fast. 50ms is enough for the
	// loop to make several attempts at a 10ms interval.
	err := waitForHealthyWith(context.Background(), probe,
		10*time.Millisecond /* interval */, 10*time.Millisecond /* start_interval */, 1 /* retries */, 50*time.Millisecond /* start_period */, 5*time.Second /* timeout */)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if calls < 2 {
		t.Fatalf("probe ran %d times; expected multiple grace attempts + one post-grace", calls)
	}
}

// TestWaitForHealthy_RetriesHonored makes sure the post-grace phase
// actually runs exactly `retries` attempts before giving up.
func TestWaitForHealthy_RetriesHonored(t *testing.T) {
	t.Parallel()

	var calls int
	probe := func(ctx context.Context, timeout time.Duration) error {
		calls++
		return fmt.Errorf("nope")
	}

	err := waitForHealthyWith(context.Background(), probe,
		1*time.Millisecond /* interval */, 1*time.Millisecond /* start_interval */, 3 /* retries */, 0 /* start_period */, time.Second /* timeout */)
	if err == nil {
		t.Fatal("expected failure")
	}
	if calls != 3 {
		t.Fatalf("probe ran %d times, want 3 retries", calls)
	}
}

func TestHealthcheckRetriesError(t *testing.T) {
	t.Parallel()

	cause := errors.New("probe failed")
	err := healthcheckRetriesError(3, 2*time.Second, cause)
	assertErrorContains(t, err, "healthcheck failed after 3 retries (start_period 2s): probe failed")
	if !errors.Is(err, cause) {
		t.Fatalf("healthcheckRetriesError does not wrap cause: %v", err)
	}
}

func TestHealthGraceSleep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		interval  time.Duration
		remaining time.Duration
		want      time.Duration
	}{
		{name: "interval fits", interval: 10 * time.Millisecond, remaining: 50 * time.Millisecond, want: 10 * time.Millisecond},
		{name: "cap to remaining", interval: 50 * time.Millisecond, remaining: 10 * time.Millisecond, want: 10 * time.Millisecond},
		{name: "zero remaining", interval: 10 * time.Millisecond, remaining: 0, want: 0},
		{name: "negative remaining", interval: 10 * time.Millisecond, remaining: -time.Millisecond, want: -time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := healthGraceSleep(tt.interval, tt.remaining); got != tt.want {
				t.Fatalf("healthGraceSleep(%s, %s) = %s, want %s", tt.interval, tt.remaining, got, tt.want)
			}
		})
	}
}

func TestHealthTimingFromSeconds(t *testing.T) {
	t.Parallel()

	got := healthTimingFromSeconds(2, 3, 1, 4)
	if got.interval != 2*time.Second {
		t.Fatalf("interval = %s, want 2s", got.interval)
	}
	if got.startInterval != time.Second {
		t.Fatalf("startInterval = %s, want 1s", got.startInterval)
	}
	if got.startPeriod != 3*time.Second {
		t.Fatalf("startPeriod = %s, want 3s", got.startPeriod)
	}
	if got.timeout != 4*time.Second {
		t.Fatalf("timeout = %s, want 4s", got.timeout)
	}

	fallback := healthTimingFromSeconds(2, 3, 0, 4)
	if fallback.startInterval != 2*time.Second {
		t.Fatalf("fallback startInterval = %s, want interval 2s", fallback.startInterval)
	}
}

func TestWaitForHealthy_NonPositiveRetriesStillProbeOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		retries int
	}{
		{name: "zero", retries: 0},
		{name: "negative", retries: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls int
			probe := func(ctx context.Context, timeout time.Duration) error {
				calls++
				return fmt.Errorf("nope")
			}

			err := waitForHealthyWith(context.Background(), probe,
				time.Millisecond, time.Millisecond, tt.retries, 0, time.Second)
			if err == nil {
				t.Fatal("expected failure")
			}
			if calls != 1 {
				t.Fatalf("probe ran %d times, want 1", calls)
			}
		})
	}
}

// TestWaitForHealthy_SucceedsDuringGrace short-circuits the loop as
// soon as the probe reports healthy inside the grace window.
func TestWaitForHealthy_SucceedsDuringGrace(t *testing.T) {
	t.Parallel()

	var calls int
	probe := func(ctx context.Context, timeout time.Duration) error {
		calls++
		if calls == 2 {
			return nil
		}
		return fmt.Errorf("not yet")
	}

	err := waitForHealthyWith(context.Background(), probe,
		1*time.Millisecond, time.Millisecond, 3, time.Second, time.Second)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if calls != 2 {
		t.Fatalf("probe ran %d times; expected early return at 2", calls)
	}
}

func TestWaitForHealthy_UsesStartIntervalDuringGrace(t *testing.T) {
	t.Parallel()

	var calls int
	probe := func(ctx context.Context, timeout time.Duration) error {
		calls++
		return fmt.Errorf("not yet")
	}

	err := waitForHealthyWith(context.Background(), probe,
		200*time.Millisecond /* interval */, 5*time.Millisecond /* start_interval */, 1 /* retries */, 25*time.Millisecond /* start_period */, time.Second)
	if err == nil {
		t.Fatal("expected failure")
	}
	if calls < 4 {
		t.Fatalf("probe ran %d times; want multiple start_interval attempts during grace", calls)
	}
}

// writeTempPrivateKey emits a throwaway valid key so probeHealthcheck's
// key-loading branch executes before the dial even starts; keeps the
// TestProbeHealthcheck_DialFailure case focused on the dial error path.
func writeTempPrivateKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return writeClientKey(t, priv)
}
