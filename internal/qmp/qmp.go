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

// Client is a connected QMP session over a unix socket.
type Client struct {
	conn net.Conn
	rd   *bufio.Reader
}

// Dial connects to the QMP socket at path, reads the greeting, negotiates
// capabilities, and returns a ready client. The timeout bounds the initial
// connect and the full handshake.
func Dial(path string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("unix", path, timeout)
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

	if err := c.execute("qmp_capabilities", nil); err != nil {
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
// halt — the caller reaps the QEMU process via its own exit detection,
// because the time from ACPI request to clean shutdown depends entirely
// on the guest OS.
func (c *Client) Powerdown(timeout time.Duration) error {
	if err := c.conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	defer func() { _ = c.conn.SetDeadline(time.Time{}) }()
	return c.execute("system_powerdown", nil)
}

// Close closes the QMP session.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

type qmpError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

// response models the three message shapes QMP may deliver on the same
// stream: a command reply (Return set), a command error (Error set), or an
// asynchronous event (Event set).
type response struct {
	Return *json.RawMessage `json:"return,omitempty"`
	Error  *qmpError        `json:"error,omitempty"`
	Event  string           `json:"event,omitempty"`
}

type command struct {
	Execute   string `json:"execute"`
	Arguments any    `json:"arguments,omitempty"`
}

// execute marshals and sends a single command, then reads responses until
// it sees either a matching return or an error. Events are skipped so the
// caller never has to deal with them.
func (c *Client) execute(cmd string, args any) error {
	payload, err := json.Marshal(command{Execute: cmd, Arguments: args})
	if err != nil {
		return fmt.Errorf("qmp encode %s: %w", cmd, err)
	}
	payload = append(payload, '\n')
	if _, err := c.conn.Write(payload); err != nil {
		return fmt.Errorf("qmp write %s: %w", cmd, err)
	}

	for {
		var resp response
		if err := c.readMessage(&resp); err != nil {
			return fmt.Errorf("qmp read response for %s: %w", cmd, err)
		}
		if resp.Event != "" {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("qmp %s: %s: %s", cmd, resp.Error.Class, resp.Error.Desc)
		}
		if resp.Return != nil {
			return nil
		}
		return fmt.Errorf("qmp %s: malformed response", cmd)
	}
}

func (c *Client) readMessage(v any) error {
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}
