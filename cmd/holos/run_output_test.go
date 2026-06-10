package main

import (
	"bytes"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestRunNextSteps(t *testing.T) {
	t.Parallel()

	gotCommands := make([]string, 0, len(runNextSteps))
	gotDescriptions := make(map[string]string, len(runNextSteps))
	for _, step := range runNextSteps {
		gotCommands = append(gotCommands, step.Command)
		gotDescriptions[step.Command] = step.Description
	}

	wantCommands := []string{"exec", "console", "logs", "down"}
	assertStringSliceEqual(t, "runNextSteps commands", gotCommands, wantCommands)
	if got := gotDescriptions["exec"]; got != "interactive shell over ssh (recommended)" {
		t.Fatalf("exec description = %q", got)
	}
	if got := gotDescriptions["down"]; got != "" {
		t.Fatalf("down description = %q, want empty", got)
	}
}

func TestWriteRunSummary(t *testing.T) {
	t.Parallel()

	record := testProjectRecord("demo",
		runtime.ServiceRecord{
			Name:            "vm",
			DesiredReplicas: 1,
			Instances: []runtime.InstanceRecord{
				{Name: "vm-0", Status: runtime.InstanceStatusRunning, PID: 321, LogPath: "/tmp/vm.log"},
			},
		},
	)

	var out bytes.Buffer
	writeRunSummary(&out, record, "/tmp/holos-demo.yaml", "demo", "debian")
	want := "project: demo\n\n" +
		"service: vm (1/1 running)\n" +
		"  INSTANCE  STATUS   PID  PORTS  LOG\n" +
		"  vm-0      running  321  -      /tmp/vm.log\n\n" +
		"compose file: /tmp/holos-demo.yaml\n" +
		"login user:   debian (cloud-init may take ~30s on first boot)\n\n" +
		"next steps:\n" +
		"  holos exec    demo     # interactive shell over ssh (recommended)\n" +
		"  holos console demo     # serial console for boot/kernel logs\n" +
		"  holos logs    demo     # console.log tail\n" +
		"  holos down    demo\n"
	if got := out.String(); got != want {
		t.Fatalf("run summary = %q, want %q", got, want)
	}
}
