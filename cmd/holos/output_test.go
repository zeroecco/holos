package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/zeroecco/holos/internal/qemu"
	"github.com/zeroecco/holos/internal/runtime"
)

func TestWriteWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		args   []any
		want   string
	}{
		{
			name:   "string",
			format: "%s",
			args:   []any{"skipping missing device"},
			want:   "warning: skipping missing device\n",
		},
		{
			name:   "formatted error",
			format: "%v; attempting ssh anyway",
			args:   []any{errors.New("ssh unavailable")},
			want:   "warning: ssh unavailable; attempting ssh anyway\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			writeWarning(&out, tt.format, tt.args...)
			if got := out.String(); got != tt.want {
				t.Fatalf("warning output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewTableWriterAlignsColumns(t *testing.T) {
	t.Parallel()

	if tableTabWidth != 8 || tablePadding != 2 || tablePadChar != ' ' || tableFlags != 0 {
		t.Fatalf("unexpected table writer settings: tabWidth=%d padding=%d padChar=%q flags=%d",
			tableTabWidth, tablePadding, tablePadChar, tableFlags)
	}

	var out bytes.Buffer
	writer := newTableWriter(&out)
	fmt.Fprintln(writer, "A\tB")
	fmt.Fprintln(writer, "long\tvalue")
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got, want := out.String(), "A     B\nlong  value\n"; got != want {
		t.Fatalf("table output = %q, want %q", got, want)
	}
}

func TestFormatProjectStatusRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		inst runtime.InstanceRecord
		want string
	}{
		{
			name: "with ports and log",
			inst: runtime.InstanceRecord{
				Name:    "web-0",
				Status:  runtime.InstanceStatusRunning,
				PID:     1234,
				LogPath: "/tmp/web.log",
				Ports: []qemu.PortMapping{{
					HostAddr:  "127.0.0.1",
					HostPort:  8080,
					GuestPort: 80,
					Protocol:  "tcp",
				}},
			},
			want: "  web-0\trunning\t1234\t127.0.0.1:8080->80/tcp\t/tmp/web.log",
		},
		{
			name: "with placeholders",
			inst: runtime.InstanceRecord{
				Name:   "web-0",
				Status: runtime.InstanceStatusStopped,
			},
			want: "  web-0\tstopped\t0\t-\t-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatProjectStatusRow(tt.inst); got != tt.want {
				t.Fatalf("formatProjectStatusRow = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteProjectStatus(t *testing.T) {
	t.Parallel()

	record := testProjectRecord("demo",
		runtime.ServiceRecord{
			Name:            "web",
			DesiredReplicas: 2,
			Instances: []runtime.InstanceRecord{
				{Name: "web-0", Status: runtime.InstanceStatusRunning, PID: 123, LogPath: "/tmp/web-0.log"},
				{Name: "web-1", Status: runtime.InstanceStatusStopped},
			},
		},
		runtime.ServiceRecord{
			Name:            "db",
			DesiredReplicas: 1,
			Instances: []runtime.InstanceRecord{
				{Name: "db-0", Status: runtime.InstanceStatusRunning, PID: 456},
			},
		},
	)

	var out bytes.Buffer
	writeProjectStatus(&out, record)
	want := "project: demo\n\n" +
		"service: web (1/2 running)\n" +
		"  INSTANCE  STATUS   PID  PORTS  LOG\n" +
		"  web-0     running  123  -      /tmp/web-0.log\n" +
		"  web-1     stopped  0    -      -\n\n" +
		"service: db (1/1 running)\n" +
		"  INSTANCE  STATUS   PID  PORTS  LOG\n" +
		"  db-0      running  456  -      -\n\n"
	if got := out.String(); got != want {
		t.Fatalf("project status output = %q, want %q", got, want)
	}
}
