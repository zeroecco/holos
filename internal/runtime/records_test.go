package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/config"
	"github.com/zeroecco/holos/internal/qemu"
)

const (
	testRecordsHTTPHostPort   = 8080
	testRecordsHTTPGuestPort  = 80
	testRecordsAdminHostAddr  = "127.0.0.2"
	testRecordsAdminHostPort  = 9000
	testRecordsAdminGuestAddr = "10.0.2.15"
	testRecordsAdminGuestPort = 9000
	testRecordsWildcardAddr   = "0.0.0.0"
)

func TestRunningCountUsesInstanceStatus(t *testing.T) {
	t.Parallel()

	service := ServiceRecord{
		Instances: []InstanceRecord{
			{Status: InstanceStatusRunning},
			{Status: InstanceStatusStopped},
			{Status: ""},
		},
	}
	if got, want := service.RunningCount(), 1; got != want {
		t.Fatalf("RunningCount = %d, want %d", got, want)
	}
}

func TestServiceRecordPortSummary(t *testing.T) {
	t.Parallel()

	var empty ServiceRecord
	if got := empty.PortSummary(); got != noPortsSummary {
		t.Fatalf("empty service PortSummary = %q, want %q", got, noPortsSummary)
	}

	service := ServiceRecord{
		Instances: []InstanceRecord{
			{},
		},
	}
	if got := service.PortSummary(); got != noPortsSummary {
		t.Fatalf("service PortSummary with no mapped ports = %q, want %q", got, noPortsSummary)
	}

	service.Instances[0].Ports = []qemu.PortMapping{{HostPort: testRecordsHTTPHostPort, GuestPort: testRecordsHTTPGuestPort, Protocol: config.DefaultProtocol}}
	if got, want := service.PortSummary(), "127.0.0.1:8080->80/tcp"; got != want {
		t.Fatalf("service PortSummary = %q, want %q", got, want)
	}
}

func TestInstanceRecordPortSummaryFormatsMultiplePorts(t *testing.T) {
	t.Parallel()

	instance := InstanceRecord{
		Ports: []qemu.PortMapping{
			{HostAddr: testRecordsWildcardAddr, HostPort: testRecordsHTTPHostPort, GuestPort: testRecordsHTTPGuestPort, Protocol: config.DefaultProtocol},
			{HostAddr: testRecordsAdminHostAddr, HostPort: testRecordsAdminHostPort, GuestAddr: testRecordsAdminGuestAddr, GuestPort: testRecordsAdminGuestPort, Protocol: config.DefaultProtocol},
		},
	}
	want := "0.0.0.0:8080->80/tcp,127.0.0.2:9000->10.0.2.15:9000/tcp"
	if got := instance.PortSummary(); got != want {
		t.Fatalf("PortSummary = %q, want %q", got, want)
	}
}
