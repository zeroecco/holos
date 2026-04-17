// Mock qemu-system-x86_64 used by the holos integration test suite.
//
// The runtime asserts liveness by checking that /proc/<pid>/comm starts with
// "qemu-". Building this program as "qemu-system-x86_64" gives a comm value
// of "qemu-system-x8" (Linux truncates to TASK_COMM_LEN=16), which satisfies
// the prefix check.
//
// Behavior:
//   - Writes the full argv to a log file (path from HOLOS_MOCK_QEMU_LOG if
//     set, otherwise <first -name value>.args in the current dir).
//   - Creates the chardev socket files listed in -chardev so tests can
//     observe their existence.
//   - Sleeps until SIGTERM/SIGKILL, imitating a long-running VM process.
package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	args := os.Args[1:]

	if logPath := os.Getenv("HOLOS_MOCK_QEMU_LOG"); logPath != "" {
		appendArgs(logPath, args)
	}

	createSocketsFromChardevs(args)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	if delayStr := os.Getenv("HOLOS_MOCK_QEMU_EXIT_AFTER"); delayStr != "" {
		if delay, err := time.ParseDuration(delayStr); err == nil {
			select {
			case <-time.After(delay):
				os.Exit(1)
			case <-signals:
				os.Exit(0)
			}
		}
	}

	<-signals
}

func appendArgs(path string, args []string) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, strings.Join(args, "\x00"))
}

// createSocketsFromChardevs scans -chardev arguments for socket paths and
// opens a listening UNIX socket on each one. The holos runtime occasionally
// checks for the existence of serial/qmp sockets after launch.
func createSocketsFromChardevs(args []string) {
	for i, a := range args {
		if a != "-chardev" || i+1 >= len(args) {
			continue
		}
		parts := strings.Split(args[i+1], ",")
		if len(parts) == 0 || !strings.HasPrefix(parts[0], "socket") {
			continue
		}
		var path string
		for _, p := range parts {
			if strings.HasPrefix(p, "path=") {
				path = strings.TrimPrefix(p, "path=")
			}
		}
		if path == "" {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		ln, err := net.Listen("unix", path)
		if err == nil {
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
