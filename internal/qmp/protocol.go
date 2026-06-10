package qmp

import (
	"encoding/json"
	"fmt"
)

const qmpMessageTerminator = '\n'

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

func qmpCommandPayload(cmd string, args any) ([]byte, error) {
	payload, err := json.Marshal(command{Execute: cmd, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("qmp encode %s: %w", cmd, err)
	}
	return append(payload, qmpMessageTerminator), nil
}

// execute marshals and sends a single command, then reads responses until
// it sees either a matching return or an error. Events are skipped so the
// caller never has to deal with them.
func (c *Client) execute(cmd string, args any) error {
	payload, err := qmpCommandPayload(cmd, args)
	if err != nil {
		return err
	}
	if _, err := c.conn.Write(payload); err != nil {
		return fmt.Errorf("qmp write %s: %w", cmd, err)
	}

	for {
		var resp response
		if err := c.readMessage(&resp); err != nil {
			return fmt.Errorf("qmp read response for %s: %w", cmd, err)
		}
		done, err := handleCommandResponse(cmd, resp)
		if err != nil {
			return err
		}
		if !done {
			continue
		}
		return nil
	}
}

func handleCommandResponse(cmd string, resp response) (done bool, err error) {
	if resp.Event != "" {
		return false, nil
	}
	if resp.Error != nil {
		return true, fmt.Errorf("qmp %s: %s: %s", cmd, resp.Error.Class, resp.Error.Desc)
	}
	if resp.Return != nil {
		return true, nil
	}
	return true, fmt.Errorf("qmp %s: malformed response", cmd)
}
