package main

import (
	"fmt"
	"net"
	"time"

	"github.com/zeroecco/holos/internal/runtime"
)

const (
	sshdProbeNetwork        = "tcp"
	sshdProbeTimeout        = 2 * time.Second
	sshdBannerPrefix        = "SSH-"
	sshdBannerProbeBytes    = len(sshdBannerPrefix)
	sshdReadyInitialBackoff = 200 * time.Millisecond
	sshdReadyMaxBackoff     = 2 * time.Second
)

func sshdReady(addr string) bool {
	conn, err := net.DialTimeout(sshdProbeNetwork, addr, sshdProbeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(sshdProbeTimeout))
	buf := make([]byte, sshdBannerProbeBytes)
	n, err := conn.Read(buf)
	if !sshdBannerReadComplete(n, err) {
		return false
	}
	return string(buf) == sshdBannerPrefix
}

func sshdBannerReadComplete(n int, err error) bool {
	return err == nil && n >= sshdBannerProbeBytes
}

func waitForSSHReady(addr string, total time.Duration) error {
	deadline := time.Now().Add(total)
	delay := sshdReadyInitialBackoff
	for time.Now().Before(deadline) {
		if sshdReady(addr) {
			return nil
		}
		time.Sleep(delay)
		delay = nextSSHReadyBackoff(delay)
	}
	return fmt.Errorf("sshd on %s did not become ready within %s", addr, total)
}

func nextSSHReadyBackoff(delay time.Duration) time.Duration {
	if delay >= sshdReadyMaxBackoff {
		return delay
	}
	next := delay * 2
	if next > sshdReadyMaxBackoff {
		return sshdReadyMaxBackoff
	}
	return next
}

func instanceIsRunning(inst runtime.InstanceRecord) bool {
	return inst.Status == runtime.InstanceStatusRunning
}

func instanceTargetRequiredError(command string) error {
	return fmt.Errorf("%s requires a project name or an instance name (e.g. \"my-stack\" or \"web-0\")", command)
}

func instanceNotRunningError(inst runtime.InstanceRecord) error {
	return fmt.Errorf("instance %q is %s", inst.Name, inst.Status)
}

func instanceSupportsExec(inst runtime.InstanceRecord) bool {
	return inst.SSHPort != 0
}

func instanceMissingExecSupportError(inst runtime.InstanceRecord) error {
	return fmt.Errorf("instance %q has no ssh port (created before exec support; recreate the stack)", inst.Name)
}

func instanceSupportsConsole(inst runtime.InstanceRecord) bool {
	return inst.SerialPath != ""
}

func instanceMissingConsoleSupportError(inst runtime.InstanceRecord) error {
	return fmt.Errorf("instance %q has no serial console (created before console support)", inst.Name)
}
