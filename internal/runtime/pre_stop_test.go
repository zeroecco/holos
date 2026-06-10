package runtime

import (
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testPreStopProject = "prestop"
	testPreStopUser    = "tester"
)

func TestRunPreStopCommandsSuccess(t *testing.T) {
	addr, keyPath, stop := startFakeSSHServer(t, 0)
	defer stop()

	manager := testPreStopManager(t, keyPath)
	service := ServiceRecord{
		LoginUser:       testPreStopUser,
		PreStopCommands: []string{"echo stopping", "touch /tmp/stopped"},
	}
	instance := InstanceRecord{
		Name:    testInstanceName(testWebServiceName, 0),
		Status:  InstanceStatusRunning,
		SSHPort: testPortFromAddr(t, addr),
	}

	if err := manager.runPreStopCommands(testPreStopProject, service, instance); err != nil {
		t.Fatalf("runPreStopCommands: %v", err)
	}
}

func TestRunPreStopCommandsSkipsEmptyAndStopped(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	running := InstanceRecord{Name: testInstanceName(testWebServiceName, 0), Status: InstanceStatusRunning}
	if err := manager.runPreStopCommands(testPreStopProject, ServiceRecord{}, running); err != nil {
		t.Fatalf("empty commands: %v", err)
	}

	stopped := InstanceRecord{Name: testInstanceName(testWebServiceName, 0), Status: InstanceStatusStopped}
	service := ServiceRecord{PreStopCommands: []string{"echo stopping"}}
	if err := manager.runPreStopCommands(testPreStopProject, service, stopped); err != nil {
		t.Fatalf("stopped instance: %v", err)
	}
}

func TestRunPreStopCommandsRequiresSSHPort(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	service := ServiceRecord{PreStopCommands: []string{"echo stopping"}}
	instance := InstanceRecord{Name: testInstanceName(testWebServiceName, 0), Status: InstanceStatusRunning}

	err := manager.runPreStopCommands(testPreStopProject, service, instance)
	assertErrorContains(t, err, "has no ssh port")
}

func TestStopServiceInstancesRunsPreStopAndMarksStopped(t *testing.T) {
	addr, keyPath, stop := startFakeSSHServer(t, 0)
	defer stop()

	manager := testPreStopManager(t, keyPath)
	service := &ServiceRecord{
		Name:            testWebServiceName,
		LoginUser:       testPreStopUser,
		PreStopCommands: []string{"echo stopping"},
		Instances: []InstanceRecord{
			{
				Name:    testInstanceName(testWebServiceName, 0),
				Status:  InstanceStatusRunning,
				SSHPort: testPortFromAddr(t, addr),
			},
		},
	}

	before := time.Now().UTC()
	if err := manager.stopServiceInstances(testPreStopProject, service); err != nil {
		t.Fatalf("stopServiceInstances: %v", err)
	}
	after := time.Now().UTC()

	instance := service.Instances[0]
	if instance.Status != InstanceStatusStopped {
		t.Fatalf("status = %q, want %q", instance.Status, InstanceStatusStopped)
	}
	assertTimeBetween(t, "LastExitTime", instance.LastExitTime, before, after)
}

func TestPreStopUser(t *testing.T) {
	t.Parallel()

	if got := preStopUser(ServiceRecord{LoginUser: testPreStopUser}); got != testPreStopUser {
		t.Fatalf("preStopUser explicit = %q, want %q", got, testPreStopUser)
	}
	if got := preStopUser(ServiceRecord{}); got != config.DefaultUser {
		t.Fatalf("preStopUser default = %q, want %q", got, config.DefaultUser)
	}
}

func testPreStopManager(t *testing.T, keyPath string) *Manager {
	t.Helper()

	stateDir := t.TempDir()
	if err := os.MkdirAll(sshDir(stateDir, testPreStopProject), sshDirPerm); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read test ssh key: %v", err)
	}
	if err := os.WriteFile(privateKeyPath(stateDir, testPreStopProject), key, sshPrivateKeyPerm); err != nil {
		t.Fatalf("write project ssh key: %v", err)
	}
	return NewManager(stateDir)
}

func testPortFromAddr(t *testing.T, addr string) int {
	t.Helper()

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return parsed
}
