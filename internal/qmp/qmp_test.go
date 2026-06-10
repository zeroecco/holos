package qmp

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testQMPGreeting = `{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}`

func listenTestSocket(t *testing.T) (string, net.Listener) {
	t.Helper()

	dir, err := os.MkdirTemp("", "qmp-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socketPath := filepath.Join(dir, "qmp.sock")
	ln, err := net.Listen(qmpUnixNetwork, socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return socketPath, ln
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

// fakeServer starts a QMP-speaking unix socket in a temp dir and returns
// its path plus a channel that receives every command the client issues.
func fakeServer(t *testing.T, greeting string, reply string) (socketPath string, commands <-chan string) {
	t.Helper()

	socketPath, ln := listenTestSocket(t)

	cmds := make(chan string, 8)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				if _, err := c.Write([]byte(greeting + "\n")); err != nil {
					return
				}
				rd := bufio.NewReader(c)
				for {
					line, err := rd.ReadBytes('\n')
					if err != nil {
						return
					}
					var cmd struct {
						Execute string `json:"execute"`
					}
					_ = json.Unmarshal(line, &cmd)
					cmds <- cmd.Execute
					if _, err := c.Write([]byte(reply + "\n")); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return socketPath, cmds
}

func TestQMPCommandPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      string
		args     any
		validate func(*testing.T, []byte)
	}{
		{
			name: "omits nil arguments",
			cmd:  qmpPowerdownCommand,
			validate: func(t *testing.T, payload []byte) {
				t.Helper()

				const want = `{"execute":"system_powerdown"}` + "\n"
				if string(payload) != want {
					t.Fatalf("payload = %q, want %q", string(payload), want)
				}
			},
		},
		{
			name: "includes arguments",
			cmd:  "device_add",
			args: map[string]string{"driver": "virtio-blk"},
			validate: func(t *testing.T, payload []byte) {
				t.Helper()

				if !strings.HasSuffix(string(payload), "\n") {
					t.Fatalf("payload must be newline terminated: %q", string(payload))
				}

				var decoded struct {
					Execute   string            `json:"execute"`
					Arguments map[string]string `json:"arguments"`
				}
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("decode payload: %v", err)
				}
				if decoded.Execute != "device_add" {
					t.Fatalf("execute = %q, want device_add", decoded.Execute)
				}
				if decoded.Arguments["driver"] != "virtio-blk" {
					t.Fatalf("arguments = %#v, want driver virtio-blk", decoded.Arguments)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := qmpCommandPayload(tt.cmd, tt.args)
			if err != nil {
				t.Fatalf("payload: %v", err)
			}
			tt.validate(t, payload)
		})
	}
}

func TestHandleCommandResponse(t *testing.T) {
	t.Parallel()

	rawReturn := json.RawMessage(`{}`)
	tests := []struct {
		name     string
		resp     response
		wantDone bool
		wantErr  string
	}{
		{name: "event", resp: response{Event: "WAKEUP"}, wantDone: false},
		{name: "return", resp: response{Return: &rawReturn}, wantDone: true},
		{name: "error", resp: response{Error: &qmpError{Class: "GenericError", Desc: "boom"}}, wantDone: true, wantErr: "GenericError: boom"},
		{name: "malformed", resp: response{}, wantDone: true, wantErr: "malformed response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			done, err := handleCommandResponse(qmpPowerdownCommand, tt.resp)
			if done != tt.wantDone {
				t.Fatalf("done = %v, want %v", done, tt.wantDone)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDial_Powerdown(t *testing.T) {
	t.Parallel()

	socket, cmds := fakeServer(t, testQMPGreeting, `{"return":{}}`)

	client, err := Dial(socket, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if got := <-cmds; got != qmpCapabilitiesCommand {
		t.Fatalf("expected %s first; got %q", qmpCapabilitiesCommand, got)
	}

	if err := client.Powerdown(1 * time.Second); err != nil {
		t.Fatalf("powerdown: %v", err)
	}
	if got := <-cmds; got != qmpPowerdownCommand {
		t.Fatalf("expected %s; got %q", qmpPowerdownCommand, got)
	}
}

func TestClientClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		client *Client
	}{
		{name: "nil client"},
		{name: "nil connection", client: &Client{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.client.Close(); err != nil {
				t.Fatalf("Close() = %v, want nil", err)
			}
		})
	}

	conn, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	client := &Client{conn: conn}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if _, err := conn.Write([]byte("x")); err == nil {
		t.Fatal("write to closed connection succeeded")
	}
}

func TestDial_RejectsEmptyGreeting(t *testing.T) {
	t.Parallel()

	socket, _ := fakeServer(t, `{"unexpected":true}`, `{"return":{}}`)

	_, err := Dial(socket, 1*time.Second)
	assertErrorContains(t, err, "empty greeting")
}

func TestDial_RejectsMalformedGreeting(t *testing.T) {
	t.Parallel()

	socket, _ := fakeServer(t, `{"QMP":`, `{"return":{}}`)

	_, err := Dial(socket, 1*time.Second)
	assertErrorContains(t, err, "qmp greeting")
}

// TestExecute_SkipsEvents confirms async events interleaved with a reply
// are transparently skipped.
func TestExecute_SkipsEvents(t *testing.T) {
	t.Parallel()

	socket, ln := listenTestSocket(t)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(testQMPGreeting + "\n"))
		rd := bufio.NewReader(conn)
		for {
			line, err := rd.ReadBytes('\n')
			if err != nil {
				return
			}
			var cmd struct {
				Execute string `json:"execute"`
			}
			_ = json.Unmarshal(line, &cmd)

			// Interleave two events before the actual reply.
			conn.Write([]byte(`{"event":"WAKEUP","timestamp":{"seconds":1,"microseconds":0}}` + "\n"))
			conn.Write([]byte(`{"event":"RESET","timestamp":{"seconds":1,"microseconds":0}}` + "\n"))
			conn.Write([]byte(`{"return":{}}` + "\n"))
			_ = cmd
		}
	}()

	client, err := Dial(socket, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	if err := client.Powerdown(1 * time.Second); err != nil {
		t.Fatalf("powerdown with events: %v", err)
	}
}

func TestExecute_PropagatesQMPError(t *testing.T) {
	t.Parallel()

	// Reply with an error the second time (the first ACK is for
	// qmp_capabilities during Dial).
	socket, ln := listenTestSocket(t)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(testQMPGreeting + "\n"))
		rd := bufio.NewReader(conn)
		first := true
		for {
			if _, err := rd.ReadBytes('\n'); err != nil {
				return
			}
			if first {
				conn.Write([]byte(`{"return":{}}` + "\n"))
				first = false
				continue
			}
			conn.Write([]byte(`{"error":{"class":"GenericError","desc":"no can do"}}` + "\n"))
		}
	}()

	client, err := Dial(socket, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	err = client.Powerdown(1 * time.Second)
	assertErrorContains(t, err, "no can do")
}

func TestExecute_RejectsMalformedResponse(t *testing.T) {
	t.Parallel()

	socket, ln := listenTestSocket(t)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(testQMPGreeting + "\n"))
		rd := bufio.NewReader(conn)
		first := true
		for {
			if _, err := rd.ReadBytes('\n'); err != nil {
				return
			}
			if first {
				conn.Write([]byte(`{"return":{}}` + "\n"))
				first = false
				continue
			}
			conn.Write([]byte(`{"unexpected":true}` + "\n"))
		}
	}()

	client, err := Dial(socket, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	err = client.Powerdown(1 * time.Second)
	assertErrorContains(t, err, "malformed response")
}

func TestDial_MissingSocket(t *testing.T) {
	t.Parallel()

	_, err := Dial(filepath.Join(t.TempDir(), "does-not-exist"), 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial to fail on missing socket")
	}
}
