package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/zeroecco/holos/internal/runtime"
)

const (
	defaultExecLoginUser   = "ubuntu"
	loopbackHost           = "127.0.0.1"
	sshIdentityFlag        = "-i"
	sshPortFlag            = "-p"
	sshOptionFlag          = "-o"
	sshTTYFlag             = "-t"
	sshStrictHostKeyOption = "StrictHostKeyChecking=no"
	sshKnownHostsOption    = "UserKnownHostsFile=/dev/null"
	sshLogLevelOption      = "LogLevel=ERROR"
)

func runExec(args []string) error {
	flags := newFlagSet("exec")
	projectFlags := addProjectFlags(flags, "")
	user := flags.String("u", "", "override login user (default: service's cloud-init user)")
	wait := flags.Duration("w", 60*time.Second, "wait up to this long for sshd to be ready (0 disables)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return instanceTargetRequiredError("exec")
	}

	manager := runtime.NewManager(*projectFlags.stateDir)
	tgt, err := resolveInstanceTarget(manager, *projectFlags.filePath, *projectFlags.stateDir, flags.Args())
	if err != nil {
		return err
	}
	if !instanceIsRunning(tgt.Inst) {
		return instanceNotRunningError(tgt.Inst)
	}
	if !instanceSupportsExec(tgt.Inst) {
		return instanceMissingExecSupportError(tgt.Inst)
	}

	loginUser := execLoginUser(*user, tgt.LoginUser)

	inst := tgt.Inst
	cmd := tgt.CmdArgs
	if *wait > 0 {
		addr := net.JoinHostPort(loopbackHost, strconv.Itoa(inst.SSHPort))
		if !sshdReady(addr) {
			fmt.Fprintf(os.Stderr, "waiting up to %s for sshd on %s (cloud-init may still be regenerating host keys) ...\n", *wait, addr)
			if err := waitForSSHReady(addr, *wait); err != nil {
				printWarning("%v; attempting ssh anyway", err)
			}
		}
	}

	keyPath, err := manager.ProjectSSHKeyPath(tgt.ProjectName)
	if err != nil {
		return err
	}
	sshArgs := buildSSHArgs(keyPath, inst.SSHPort, loginUser, cmd)

	sshBin, err := exec.LookPath(sshClientBinary)
	if err != nil {
		return fmt.Errorf("%s client not found in PATH: %w", sshClientBinary, err)
	}
	argv := append([]string{sshBin}, sshArgs...)
	return syscall.Exec(sshBin, argv, os.Environ())
}

func buildSSHArgs(keyPath string, sshPort int, loginUser string, cmd []string) []string {
	sshArgs := []string{
		sshIdentityFlag, keyPath,
		sshPortFlag, strconv.Itoa(sshPort),
		sshOptionFlag, sshStrictHostKeyOption,
		sshOptionFlag, sshKnownHostsOption,
		sshOptionFlag, sshLogLevelOption,
	}
	if len(cmd) == 0 {
		sshArgs = append(sshArgs, sshTTYFlag)
	}
	sshArgs = append(sshArgs, sshLoginTarget(loginUser))
	sshArgs = append(sshArgs, cmd...)
	return sshArgs
}

func sshLoginTarget(loginUser string) string {
	return loginUser + "@" + loopbackHost
}

func execLoginUser(override, targetUser string) string {
	if override != "" {
		return override
	}
	if targetUser != "" {
		return targetUser
	}
	return defaultExecLoginUser
}
