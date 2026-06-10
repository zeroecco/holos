package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

func TestCheckStateDirCreatesPrivateWritableDir(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(t.TempDir(), "state")
	check := checkStateDir(stateDir)
	if check.Status != doctorStatusOK {
		t.Fatalf("checkStateDir status = %s (%s), want %s", check.Status, check.Message, doctorStatusOK)
	}
	assertDoctorMessageContains(t, check.Message, stateDir)

	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if got := info.Mode().Perm(); got != doctorStateDirPerm {
		t.Fatalf("state dir mode = %v, want %v", got, doctorStateDirPerm)
	}
}

func TestDoctorHasFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report doctorReport
		want   bool
	}{
		{name: "empty"},
		{
			name: "ok and warn only",
			report: doctorReport{Checks: []doctorCheck{
				{Status: doctorStatusOK},
				{Status: doctorStatusWarn},
			}},
		},
		{
			name: "has failure",
			report: doctorReport{Checks: []doctorCheck{
				{Status: doctorStatusOK},
				{Status: doctorStatusFail},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := doctorHasFailure(tt.report); got != tt.want {
				t.Fatalf("doctorHasFailure = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoctorOVMFSuccessMessage(t *testing.T) {
	t.Parallel()

	firmware := runtime.OVMFFirmware{
		CodePath:         "/usr/share/OVMF/OVMF_CODE.fd",
		VarsTemplatePath: "/usr/share/OVMF/OVMF_VARS.fd",
	}
	want := "CODE=/usr/share/OVMF/OVMF_CODE.fd VARS=/usr/share/OVMF/OVMF_VARS.fd"
	if got := doctorOVMFSuccessMessage(firmware); got != want {
		t.Fatalf("doctorOVMFSuccessMessage() = %q, want %q", got, want)
	}
}

func TestDoctorOVMFEnvOverrideConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		codeEnv string
		varsEnv string
		want    bool
	}{
		{name: "neither"},
		{name: "code only", codeEnv: "/firmware/OVMF_CODE.fd", want: true},
		{name: "vars only", varsEnv: "/firmware/OVMF_VARS.fd", want: true},
		{name: "both", codeEnv: "/firmware/OVMF_CODE.fd", varsEnv: "/firmware/OVMF_VARS.fd", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := doctorOVMFEnvOverrideConfigured(tt.codeEnv, tt.varsEnv); got != tt.want {
				t.Fatalf("doctorOVMFEnvOverrideConfigured = %v, want %v", got, tt.want)
			}
		})
	}
}
