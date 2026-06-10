package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	locksStateSubdir     = "locks"
	projectLockExtension = ".lock"
	projectLockPerm      = os.FileMode(0o600)
	projectLockOwnerLine = "pid=%d started_at=%s\n"
)

func locksDir(root string) string {
	return filepath.Join(root, locksStateSubdir)
}

func projectLockFile(root, name string) string {
	return filepath.Join(locksDir(root), name+projectLockExtension)
}

func writeProjectLockOwner(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	_, err := file.WriteString(currentProjectLockOwner())
	return err
}

func currentProjectLockOwner() string {
	return formatProjectLockOwner(os.Getpid(), time.Now().UTC())
}

func formatProjectLockOwner(pid int, startedAt time.Time) string {
	return fmt.Sprintf(projectLockOwnerLine, pid, startedAt.Format(time.RFC3339))
}

func readProjectLockOwner(file *os.File) string {
	if _, err := file.Seek(0, 0); err != nil {
		return ""
	}
	payload, err := os.ReadFile(file.Name())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}
