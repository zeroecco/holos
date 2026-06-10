package runtime

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/zeroecco/holos/internal/config"
)

// waitForServiceHealthy blocks until every replica of a service passes
// its healthcheck. Called only when a downstream service depends on
// this one. We don't want to stall `holos up` on informational
// probes that nothing is waiting for.
func (m *Manager) waitForServiceHealthy(svc *ServiceRecord, manifest config.Manifest, keyPath string) error {
	hc := manifest.Healthcheck
	if hc == nil {
		return nil
	}
	user := healthcheckUser(manifest)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, inst := range svc.Instances {
		if inst.SSHPort == 0 {
			return healthcheckMissingSSHPortError(inst)
		}
		addr := healthcheckSSHAddr(inst.SSHPort)
		if err := waitForHealthy(ctx, addr, user, keyPath,
			hc.Test, hc.IntervalSec, hc.Retries, hc.StartPeriodSec, hc.TimeoutSec); err != nil {
			return fmt.Errorf("instance %q: %w", inst.Name, err)
		}
	}
	return nil
}

func healthcheckMissingSSHPortError(inst InstanceRecord) error {
	return fmt.Errorf("instance %q has no ssh port; cannot run healthcheck", inst.Name)
}

func healthcheckUser(manifest config.Manifest) string {
	if manifest.CloudInit.User != "" {
		return manifest.CloudInit.User
	}
	return config.DefaultUser
}

func healthcheckSSHAddr(sshPort int) string {
	return net.JoinHostPort(defaultHostAddr, strconv.Itoa(sshPort))
}
