// Package qmp implements a minimal QEMU Machine Protocol client. Only the
// subset needed for graceful guest shutdown is covered: capability
// negotiation and fire-and-acknowledge commands like system_powerdown.
//
// A typical usage:
//
//	client, err := qmp.Dial(socketPath, 2*time.Second)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//	if err := client.Powerdown(2*time.Second); err != nil {
//	    return err
//	}
//
// The client reads line-delimited JSON responses and transparently skips
// asynchronous events (SHUTDOWN, POWERDOWN, etc.) that may interleave with
// command replies.
package qmp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	qmpUnixNetwork         = "unix"
	qmpCapabilitiesCommand = "qmp_capabilities"
	qmpPowerdownCommand    = "system_powerdown"
)

// Client is a connected QMP session over a unix socket.
type Client struct {
	conn net.Conn
	rd   *bufio.Reader
}

// Dial connects to the QMP socket at path, reads the greeting, negotiates
// capabilities, and returns a ready client. The timeout bounds the initial
// connect and the full handshake.
func Dial(path string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout(qmpUnixNetwork, path, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial qmp %s: %w", path, err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	c := &Client{conn: conn, rd: bufio.NewReader(conn)}

	var greeting struct {
		QMP json.RawMessage `json:"QMP"`
	}
	if err := c.readMessage(&greeting); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("qmp greeting: %w", err)
	}
	if len(greeting.QMP) == 0 {
		_ = conn.Close()
		return nil, errors.New("qmp: empty greeting")
	}

	if err := c.execute(qmpCapabilitiesCommand, nil); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

// Powerdown sends system_powerdown and waits up to timeout for the
// server's acknowledgement. It does NOT wait for the guest to actually
// halt. The caller reaps the QEMU process via its own exit detection,
// because the time from ACPI request to clean shutdown depends entirely
// on the guest OS.
func (c *Client) Powerdown(timeout time.Duration) error {
	if err := c.conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	defer func() { _ = c.conn.SetDeadline(time.Time{}) }()
	return c.execute(qmpPowerdownCommand, nil)
}

// Close closes the QMP session.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) readMessage(v any) error {
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}
