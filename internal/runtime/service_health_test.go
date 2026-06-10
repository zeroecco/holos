package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
)

const (
	testHealthInstanceName = "web-0"
)

func TestHealthcheckUser(t *testing.T) {
	t.Parallel()

	if got := healthcheckUser(config.Manifest{}); got != config.DefaultUser {
		t.Fatalf("healthcheckUser(default) = %q, want %q", got, config.DefaultUser)
	}
	if got := healthcheckUser(config.Manifest{CloudInit: config.CloudInit{User: "debian"}}); got != "debian" {
		t.Fatalf("healthcheckUser(custom) = %q, want debian", got)
	}
}

func TestHealthcheckSSHAddr(t *testing.T) {
	t.Parallel()

	if got, want := healthcheckSSHAddr(2222), "127.0.0.1:2222"; got != want {
		t.Fatalf("healthcheckSSHAddr = %q, want %q", got, want)
	}
}

func TestHealthcheckMissingSSHPortError(t *testing.T) {
	t.Parallel()

	err := healthcheckMissingSSHPortError(InstanceRecord{Name: testHealthInstanceName})
	want := `instance "web-0" has no ssh port; cannot run healthcheck`
	if got := err.Error(); got != want {
		t.Fatalf("healthcheckMissingSSHPortError = %q, want %q", got, want)
	}
}

func TestWaitForServiceHealthySkipsNilHealthcheck(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	service := testHealthServiceWithoutSSHPort()
	if err := manager.waitForServiceHealthy(service, config.Manifest{}, "/missing/key"); err != nil {
		t.Fatalf("waitForServiceHealthy(nil healthcheck) = %v, want nil", err)
	}
}

func TestWaitForServiceHealthyRequiresSSHPort(t *testing.T) {
	t.Parallel()

	manager := &Manager{}
	service := testHealthServiceWithoutSSHPort()
	manifest := config.Manifest{
		Healthcheck: &config.HealthcheckConfig{
			Test:        []string{"true"},
			IntervalSec: 1,
			Retries:     1,
			TimeoutSec:  1,
		},
	}
	err := manager.waitForServiceHealthy(service, manifest, "/missing/key")
	assertErrorContains(t, err, `instance "`+testHealthInstanceName+`" has no ssh port`)
}

func testHealthServiceWithoutSSHPort() *ServiceRecord {
	return &ServiceRecord{
		Instances: []InstanceRecord{
			{Name: testHealthInstanceName, SSHPort: 0},
		},
	}
}
