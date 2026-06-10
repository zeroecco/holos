package main

import (
	"bytes"
	"testing"
)

func TestWriteDoctorReport(t *testing.T) {
	t.Parallel()

	report := doctorReport{
		OS:       "linux",
		Arch:     "amd64",
		StateDir: "/tmp/holos-state",
		Checks: []doctorCheck{
			{Name: doctorHostOSCheckName, Status: doctorStatusOK, Message: "Linux host can run KVM workloads"},
			{Name: doctorOVMFCheckName, Status: doctorStatusWarn, Message: "OVMF firmware not found"},
		},
	}

	var out bytes.Buffer
	writeDoctorReport(&out, report)
	want := "holos doctor (linux/amd64)\n" +
		"state dir: /tmp/holos-state\n\n" +
		"CHECK          STATUS  DETAIL\n" +
		"host os        ok      Linux host can run KVM workloads\n" +
		"OVMF firmware  warn    OVMF firmware not found\n"
	if got := out.String(); got != want {
		t.Fatalf("doctor report output = %q, want %q", got, want)
	}
}
