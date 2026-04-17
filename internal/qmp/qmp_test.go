package qmp

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// fakeServer starts a QMP-speaking unix socket in a temp dir and returns
// its path plus a channel that receives every command the client issues.
func fakeServer(t *testing.T, greeting string, reply string) (socketPath string, commands <-chan string) {
	t.Helper()

	socketPath = filepath.Join(t.TempDir(), "qmp.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

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

func TestDial_Powerdown(t *testing.T) {
	t.Parallel()

	greeting := `{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}`
	socket, cmds := fakeServer(t, greeting, `{"return":{}}`)

	client, err := Dial(socket, 1*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if got := <-cmds; got != "qmp_capabilities" {
		t.Fatalf("expected qmp_capabilities first; got %q", got)
	}

	if err := client.Powerdown(1 * time.Second); err != nil {
		t.Fatalf("powerdown: %v", err)
	}
	if got := <-cmds; got != "system_powerdown" {
		t.Fatalf("expected system_powerdown; got %q", got)
	}
}

// TestExecute_SkipsEvents confirms async events interleaved with a reply
// are transparently skipped.
func TestExecute_SkipsEvents(t *testing.T) {
	t.Parallel()

	greeting := `{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}`

	socket := filepath.Join(t.TempDir(), "qmp.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(greeting + "\n"))
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

	greeting := `{"QMP":{"version":{"qemu":{"major":8,"minor":2,"micro":0}},"capabilities":[]}}`

	// Reply with an error the second time (the first ACK is for
	// qmp_capabilities during Dial).
	socket := filepath.Join(t.TempDir(), "qmp.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte(greeting + "\n"))
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
	if err == nil {
		t.Fatal("expected error from QMP server")
	}
	if got := err.Error(); !contains(got, "no can do") {
		t.Fatalf("expected error to propagate server desc; got %q", got)
	}
}

func TestDial_MissingSocket(t *testing.T) {
	t.Parallel()

	_, err := Dial(filepath.Join(t.TempDir(), "does-not-exist"), 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial to fail on missing socket")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
