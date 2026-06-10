package main

import (
	"bytes"
	"testing"

	"github.com/zeroecco/holos/internal/qemu"
	"github.com/zeroecco/holos/internal/runtime"
)

func TestWriteProjectsTableEmpty(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := writeProjectsTable(&out, nil); err != nil {
		t.Fatalf("writeProjectsTable: %v", err)
	}
	if got, want := out.String(), "no running projects\n"; got != want {
		t.Fatalf("empty projects table = %q, want %q", got, want)
	}
}

func TestWriteProjectsTable(t *testing.T) {
	t.Parallel()

	projects := []*runtime.ProjectRecord{
		testProjectRecord("alpha",
			runtime.ServiceRecord{
				Name:            "web",
				DesiredReplicas: 2,
				Instances: []runtime.InstanceRecord{
					{Status: runtime.InstanceStatusRunning},
					{Status: runtime.InstanceStatusStopped},
				},
			},
		),
		testProjectRecord("beta",
			runtime.ServiceRecord{
				Name:            "db",
				DesiredReplicas: 1,
				Instances: []runtime.InstanceRecord{
					{
						Status: runtime.InstanceStatusRunning,
						Ports: []qemu.PortMapping{{
							HostAddr:  "127.0.0.1",
							HostPort:  15432,
							GuestPort: 5432,
							Protocol:  "tcp",
						}},
					},
				},
			},
		),
	}

	var out bytes.Buffer
	if err := writeProjectsTable(&out, projects); err != nil {
		t.Fatalf("writeProjectsTable: %v", err)
	}
	want := "PROJECT  SERVICE  DESIRED  RUNNING  PORTS\n" +
		"alpha    web      2        1        -\n" +
		"beta     db       1        1        127.0.0.1:15432->5432/tcp\n"
	if got := out.String(); got != want {
		t.Fatalf("projects table = %q, want %q", got, want)
	}
}
