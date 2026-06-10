package runtime

import (
	"testing"

	"github.com/zeroecco/holos/internal/compose"
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

func TestNetworkSegmentRecordsSortsAndPersistsSegments(t *testing.T) {
	t.Parallel()

	segments := networkSegmentRecords(compose.NetworkPlan{
		Segments: map[string]compose.NetworkSegmentPlan{
			"frontend": {
				Name:           "frontend",
				MulticastGroup: "239.0.0.2",
				MulticastPort:  10002,
				Subnet:         "10.10.2.0/24",
				Hosts:          map[string]string{"web": "10.10.2.2"},
			},
			"backend": {
				Name:           "backend",
				MulticastGroup: "239.0.0.1",
				MulticastPort:  10001,
				Subnet:         "10.10.1.0/24",
				Hosts:          map[string]string{"db": "10.10.1.2"},
			},
		},
	})

	if len(segments) != 2 {
		t.Fatalf("segments len = %d, want 2: %#v", len(segments), segments)
	}
	if segments[0].Name != "backend" || segments[1].Name != "frontend" {
		t.Fatalf("segments order = %#v, want backend then frontend", segments)
	}
	if got := segments[0].Hosts["db"]; got != "10.10.1.2" {
		t.Fatalf("backend host db = %q, want 10.10.1.2", got)
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
