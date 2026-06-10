package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/zeroecco/holos/internal/config"
)

const preStopTimeout = 30 * time.Second

func (m *Manager) runPreStopCommands(project string, service ServiceRecord, instance InstanceRecord) error {
	if len(service.PreStopCommands) == 0 || instance.Status != InstanceStatusRunning {
		return nil
	}
	if instance.SSHPort == 0 {
		return fmt.Errorf("instance %q has no ssh port; cannot run pre_stop", instance.Name)
	}
	keyPath := privateKeyPath(m.stateDir, project)
	key, err := loadSSHKey(keyPath)
	if err != nil {
		return err
	}
	addr := healthcheckSSHAddr(instance.SSHPort)
	user := preStopUser(service)

	ctx, cancel := context.WithTimeout(context.Background(), preStopTimeout)
	defer cancel()

	client, err := newHealthSSHClient(ctx, addr, user, key, preStopTimeout)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, command := range service.PreStopCommands {
		if err := runSSHCommand(client, command, "pre_stop"); err != nil {
			return fmt.Errorf("command %q: %w", command, err)
		}
	}
	return nil
}

func preStopUser(service ServiceRecord) string {
	if service.LoginUser != "" {
		return service.LoginUser
	}
	return config.DefaultUser
}
