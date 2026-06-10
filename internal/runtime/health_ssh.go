package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	shellQuoteChar       = '\''
	shellQuoteEscapeSeq  = `'\''`
	shellArgSeparator    = ' '
	shellJoinInitialSize = 64
)

// loadSSHKey parses the OpenSSH private key at path.
func loadSSHKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}
	return signer, nil
}

func newHealthSSHClient(ctx context.Context, addr, user string, key ssh.Signer, timeout time.Duration) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(key)},
		// Guest host keys change every `down`/`up`; pinning them
		// would force operators to manage known_hosts for ephemeral
		// fleets. Fingerprints still gain weak protection from the
		// fact that we only dial 127.0.0.1 on a port we just bound.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	dialer := &net.Dialer{Timeout: timeout}
	rawConn, err := dialer.DialContext(ctx, tcpNetwork, addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	_ = rawConn.SetDeadline(time.Now().Add(timeout))

	clientConn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, config)
	if err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	return ssh.NewClient(clientConn, chans, reqs), nil
}

func runHealthSSHCommand(client *ssh.Client, cmd []string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stderr = &stderr
	session.Stdout = io.Discard

	if err := session.Run(shellJoin(cmd)); err != nil {
		if exit, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("healthcheck exit=%d: %s", exit.ExitStatus(), stderr.String())
		}
		return fmt.Errorf("healthcheck run: %w", err)
	}
	return nil
}

// shellJoin renders argv as a single string suitable for session.Run,
// which hands the whole string to the remote shell.
func shellJoin(argv []string) string {
	buf := make([]byte, 0, shellJoinInitialSize)
	for i, a := range argv {
		if i > 0 {
			buf = append(buf, shellArgSeparator)
		}
		buf = append(buf, shellQuoteArg(a)...)
	}
	return string(buf)
}

func shellQuoteArg(arg string) string {
	buf := make([]byte, 0, len(arg)+2)
	buf = append(buf, shellQuoteChar)
	for _, r := range arg {
		if r == shellQuoteChar {
			buf = append(buf, shellQuoteEscapeSeq...)
			continue
		}
		buf = append(buf, string(r)...)
	}
	buf = append(buf, shellQuoteChar)
	return string(buf)
}
