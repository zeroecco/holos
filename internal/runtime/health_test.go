package runtime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "exit=2") {
		t.Fatalf("error should mention exit code; got %v", err)
	}
}

// TestProbeHealthcheck_DialFailure ensures a dead port surfaces as a
// dial error rather than a panic or hang.
func TestProbeHealthcheck_DialFailure(t *testing.T) {
	t.Parallel()

	keyPath := writeTempPrivateKey(t)

	// Port 1 is never bound on macOS/Linux; connect refuses quickly.
	err := probeHealthcheck(context.Background(), "127.0.0.1:1", "tester", keyPath,
		[]string{"true"}, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Fatalf("error should mention dial; got %v", err)
	}
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
