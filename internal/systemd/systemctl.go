package systemd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemctlBinary = "systemctl"

func systemctlAvailable() bool {
	_, err := exec.LookPath(systemctlBinary)
	return err == nil
}

func systemctlReload(scope Scope) error {
	return runSystemctl(systemctlReloadArgs(scope)...)
}

func systemctlReloadArgs(scope Scope) []string {
	args := scopeArgs(scope)
	args = append(args, "daemon-reload")
	return args
}

func systemctlEnable(scope Scope, unitPath string, now bool) error {
	return runSystemctl(systemctlEnableArgs(scope, unitPath, now)...)
}

func systemctlEnableArgs(scope Scope, unitPath string, now bool) []string {
	args := scopeArgs(scope)
	args = append(args, "enable")
	if now {
		args = append(args, "--now")
	}
	// Pass the absolute path so systemctl treats it as "link in place
	// and enable", which is friendlier than requiring the unit to be
	// in a search path first.
	args = append(args, unitPath)
	return args
}

func systemctlDisable(scope Scope, unitPath string) error {
	return runSystemctl(systemctlDisableArgs(scope, unitPath)...)
}

func systemctlDisableArgs(scope Scope, unitPath string) []string {
	args := scopeArgs(scope)
	args = append(args, "disable", "--now", filepath.Base(unitPath))
	return args
}

func scopeArgs(scope Scope) []string {
	if scope == ScopeUser {
		return []string{"--user"}
	}
	return nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command(systemctlBinary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", systemctlBinary, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
