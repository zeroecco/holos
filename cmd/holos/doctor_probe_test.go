package main

import (
	"testing"
	"time"
)

func TestRunDoctorProbeReturnsFirstOutputLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: ""},
		{name: "single line", body: "printf ' qemu-img version 9.0 '\n", want: "qemu-img version 9.0"},
		{name: "multiple lines", body: "printf ' version one \\nignored\\n'\n", want: "version one"},
		{name: "crlf", body: "printf ' version one \\r\\nignored\\r\\n'\n", want: "version one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			probe := writeTestFile(t, t.TempDir(), "probe", "#!/bin/sh\n"+tt.body, 0o755)
			got, err := runDoctorProbe(probe, nil)
			if err != nil {
				t.Fatalf("runDoctorProbe: %v", err)
			}
			if got != tt.want {
				t.Fatalf("runDoctorProbe output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunDoctorProbeIncludesFirstFailureLine(t *testing.T) {
	t.Parallel()

	probe := writeTestFile(t, t.TempDir(), "probe", "#!/bin/sh\necho ' failure one '\necho failure two\nexit 7\n", 0o755)
	_, err := runDoctorProbe(probe, nil)
	if err == nil {
		t.Fatal("runDoctorProbe succeeded for failing probe")
	}
	assertErrorContains(t, err, "failure one")
	assertErrorOmits(t, err, "failure two")
}

func TestRunDoctorProbeTimeout(t *testing.T) {
	t.Parallel()

	probe := writeTestFile(t, t.TempDir(), "probe", "#!/bin/sh\nsleep 1\n", 0o755)
	_, err := runDoctorProbeWithTimeout(probe, nil, 10*time.Millisecond)
	if err == nil {
		t.Fatal("runDoctorProbeWithTimeout succeeded for timed-out probe")
	}
	if err.Error() != doctorProbeTimeoutMessage {
		t.Fatalf("runDoctorProbeWithTimeout error = %q, want %q", err, doctorProbeTimeoutMessage)
	}
}
