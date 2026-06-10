package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingCommandMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		envVar  string
		purpose string
		want    string
	}{
		{
			name:    "with env override",
			command: "qemu-system-x86_64",
			envVar:  "HOLOS_QEMU_SYSTEM",
			purpose: "runs VMs",
			want:    "runs VMs; install it or set HOLOS_QEMU_SYSTEM",
		},
		{
			name:    "without env override",
			command: "cloud-localds",
			purpose: "builds seed media",
			want:    "builds seed media; install cloud-localds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := missingCommandMessage(tt.command, tt.envVar, tt.purpose); got != tt.want {
				t.Fatalf("missingCommandMessage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckAnyCommandUsesFirstPassingCandidate(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken", "#!/bin/sh\necho broken\nexit 9\n", 0o755)
	writeTestFile(t, dir, "working", "#!/bin/sh\necho working-version\n", 0o755)
	t.Setenv("PATH", dir)

	check := checkAnyCommand("builder", []doctorCommand{
		{name: "missing", args: []string{doctorVersionFlag}},
		{name: "broken", args: []string{doctorVersionFlag}},
		{name: "working", args: []string{doctorVersionFlag}},
	}, "builds seed media")
	if check.Status != doctorStatusOK {
		t.Fatalf("checkAnyCommand status = %s (%s), want %s", check.Status, check.Message, doctorStatusOK)
	}
	assertDoctorMessageContains(t, check.Message,
		"working at "+filepath.Join(dir, "working"),
		"working-version",
	)
}

func TestCheckAnyCommandReportsAllFailures(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "broken", "#!/bin/sh\necho broken-output\nexit 9\n", 0o755)
	t.Setenv("PATH", dir)

	check := checkAnyCommand("builder", []doctorCommand{
		{name: "missing"},
		{name: "broken"},
	}, "builds seed media")
	if check.Status != doctorStatusFail {
		t.Fatalf("checkAnyCommand status = %s (%s), want %s", check.Status, check.Message, doctorStatusFail)
	}
	assertDoctorMessageContains(t, check.Message,
		"builds seed media; install one of missing, broken",
		"missing: not found",
		"broken: exit status 9: broken-output",
	)
}

func TestCheckCommandReportsOverrideAndProbeFailures(t *testing.T) {
	dir := t.TempDir()

	notExecutable := writeTestFile(t, dir, "not-executable", "#!/bin/sh\n", 0o644)
	t.Setenv("HOLOS_TEST_DOCTOR", notExecutable)
	check := checkCommand("probe", "HOLOS_TEST_DOCTOR", []string{doctorVersionFlag}, "test probe")
	if check.Status != doctorStatusFail {
		t.Fatalf("non-executable override status = %s, want %s", check.Status, doctorStatusFail)
	}
	assertDoctorMessageContains(t, check.Message,
		"HOLOS_TEST_DOCTOR points to "+notExecutable,
		"execute bit is not set",
	)

	broken := writeTestFile(t, dir, "broken", "#!/bin/sh\necho broken-output\nexit 7\n", 0o755)
	t.Setenv("HOLOS_TEST_DOCTOR", broken)
	check = checkCommand("probe", "HOLOS_TEST_DOCTOR", []string{doctorVersionFlag}, "test probe")
	if check.Status != doctorStatusFail {
		t.Fatalf("probe failure status = %s, want %s", check.Status, doctorStatusFail)
	}
	assertDoctorMessageContains(t, check.Message,
		"probe found at "+broken,
		"probe failed",
		"exit status 7",
		"broken-output",
	)
}

func TestCheckExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	executable := writeTestFile(t, dir, "executable", "#!/bin/sh\n", executableModeBits)
	if err := checkExecutable(executable); err != nil {
		t.Fatalf("checkExecutable(executable): %v", err)
	}

	notExecutable := writeTestFile(t, dir, "not-executable", "#!/bin/sh\n", 0o644)
	assertErrorContains(t, checkExecutable(notExecutable), "execute bit")

	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	assertErrorContains(t, checkExecutable(subdir), "directory")

	if err := checkExecutable(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("checkExecutable(missing) = nil, want stat error")
	}
}

func assertDoctorMessageContains(t *testing.T, message string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(message, want) {
			t.Fatalf("doctor message = %q, want substring %q", message, want)
		}
	}
}
