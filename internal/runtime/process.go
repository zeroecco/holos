package runtime

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

const (
	qemuProcessNamePrefix = "qemu-"
	procRoot              = "/proc"
	procCommFile          = "comm"
)

// processAlive reports whether pid refers to a running QEMU process we
// started. The signal-0 probe alone is insufficient because Linux PIDs are
// recycled; after a long-running state file or a host reboot, `pid` may
// point at an unrelated process. We defend against that by also checking
// that /proc/<pid>/comm starts with "qemu-". Cheap on Linux (holos is
// Linux-only) and enough to avoid ever SIGTERM'ing a stranger.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	comm, err := os.ReadFile(fmt.Sprintf("%s/%d/%s", procRoot, pid, procCommFile))
	if err != nil {
		// On non-Linux hosts /proc is absent. Fall back to the bare
		// signal check so tests and dev environments still work.
		return true
	}
	return isQEMUProcessName(string(comm))
}

func isQEMUProcessName(comm string) bool {
	return strings.HasPrefix(strings.TrimSpace(comm), qemuProcessNamePrefix)
}
