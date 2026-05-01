// Mock qemu-system-x86_64 used by the holos integration test suite.
//
// The runtime asserts liveness by checking that /proc/<pid>/comm starts with
// "qemu-". Building this program as "qemu-system-x86_64" gives a comm value
// of "qemu-system-x8" (Linux truncates to TASK_COMM_LEN=16), which satisfies
// the prefix check.
//
// Behavior:
//   - Writes the full argv to a log file (path from HOLOS_MOCK_QEMU_LOG if
//     set, otherwise not logged).
//   - Creates the chardev socket files listed in -chardev so tests can
//     observe their existence.
//   - For chardev id=qmp, speaks the QEMU Machine Protocol: sends a
//     greeting, ACKs every command, and exits 0 when system_powerdown is
//     received so tests can verify the graceful-shutdown path.
//   - Sleeps until SIGTERM/SIGKILL otherwise, imitating a long-running VM.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	args := os.Args[1:]

	if logPath := os.Getenv("HOLOS_MOCK_QEMU_LOG"); logPath != "" {
		appendArgs(logPath, args)
	}

	hostForwards := bindHostForwards(args)
	defer closeListeners(hostForwards)

	powerdown := make(chan struct{}, 1)
	createSocketsFromChardevs(args, powerdown)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	if delayStr := os.Getenv("HOLOS_MOCK_QEMU_EXIT_AFTER"); delayStr != "" {
		if delay, err := time.ParseDuration(delayStr); err == nil {
			select {
			case <-time.After(delay):
				logEvent("exit-after")
				os.Exit(1)
			case <-signals:
				logEvent("sigterm")
				os.Exit(0)
			case <-powerdown:
				logEvent("qmp-powerdown")
				os.Exit(0)
			}
		}
	}

	select {
	case <-signals:
		logEvent("sigterm")
	case <-powerdown:
		logEvent("qmp-powerdown")
		// Simulate a short shutdown latency so tests observe the guest
		// halting after the ACK rather than instantly.
		time.Sleep(50 * time.Millisecond)
	}
}

func bindHostForwards(args []string) []net.Listener {
	if os.Getenv("HOLOS_MOCK_BIND_HOSTFWD") == "" {
		return nil
	}
	ports := hostForwardPorts(args)
	listeners := make([]net.Listener, 0, len(ports))
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			fmt.Fprintf(os.Stderr, "qemu-system-x86_64: -netdev user: hostfwd: address already in use for 127.0.0.1:%d\n", port)
			closeListeners(listeners)
			os.Exit(1)
		}
		listeners = append(listeners, ln)
	}
	return listeners
}

func hostForwardPorts(args []string) []int {
	var ports []int
	const prefix = "hostfwd=tcp:127.0.0.1:"
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			if !strings.HasPrefix(part, prefix) {
				continue
			}
			rest := strings.TrimPrefix(part, prefix)
			end := strings.Index(rest, "-:")
			if end == -1 {
				continue
			}
			port, err := strconv.Atoi(rest[:end])
			if err == nil {
				ports = append(ports, port)
			}
		}
	}
	return ports
}

func closeListeners(listeners []net.Listener) {
	for _, ln := range listeners {
		_ = ln.Close()
	}
}

func appendArgs(path string, args []string) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, strings.Join(args, "\x00"))
}

func logEvent(tag string) {
	path := os.Getenv("HOLOS_MOCK_QEMU_LOG")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "EVENT:%s\n", tag)
}

// createSocketsFromChardevs opens a listening unix socket for every
// `-chardev socket,...` argument, routing the QMP socket (id=qmp) to a
// proper QMP responder and all others to a plain accept loop.
func createSocketsFromChardevs(args []string, powerdown chan<- struct{}) {
	for i, a := range args {
		if a != "-chardev" || i+1 >= len(args) {
			continue
		}
		parts := strings.Split(args[i+1], ",")
		if len(parts) == 0 || !strings.HasPrefix(parts[0], "socket") {
			continue
		}
		var (
			path string
			id   string
		)
		for _, p := range parts {
			switch {
			case strings.HasPrefix(p, "path="):
				path = strings.TrimPrefix(p, "path=")
			case strings.HasPrefix(p, "id="):
				id = strings.TrimPrefix(p, "id=")
			}
		}
		if path == "" {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		ln, err := net.Listen("unix", path)
		if err != nil {
			continue
		}
		if id == "qmp" {
			go acceptQMP(ln, powerdown)
		} else {
			go acceptForever(ln)
		}
	}
}

func acceptForever(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}
}

func acceptQMP(ln net.Listener, powerdown chan<- struct{}) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleQMP(conn, powerdown)
	}
}

func handleQMP(conn net.Conn, powerdown chan<- struct{}) {
	defer conn.Close()

	greeting := `{"QMP":{"version":{"qemu":{"major":0,"minor":0,"micro":0,"package":"holos-mock"}},"capabilities":[]}}` + "\n"
	if _, err := conn.Write([]byte(greeting)); err != nil {
		return
	}

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

		if _, err := conn.Write([]byte(`{"return":{}}` + "\n")); err != nil {
			return
		}

		if cmd.Execute == "system_powerdown" {
			// Signal the main goroutine to terminate, emulating the
			// guest completing its ACPI shutdown.
			select {
			case powerdown <- struct{}{}:
			default:
			}
			return
		}
	}
}
