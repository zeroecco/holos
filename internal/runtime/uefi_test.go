package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestOVMFFirmware(t *testing.T, dir string, varsContent string) (string, string) {
	t.Helper()

	code := filepath.Join(dir, "OVMF_CODE.fd")
	vars := filepath.Join(dir, "OVMF_VARS.fd")
	if err := os.WriteFile(code, []byte("code"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vars, []byte(varsContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return code, vars
}

func TestResolveOVMFFirmwareRequiresEnvPair(t *testing.T) {
	dir := t.TempDir()
	code, vars := writeTestOVMFFirmware(t, dir, "vars")

	t.Setenv(ovmfCodeEnv, code)
	t.Setenv(ovmfVarsEnv, "")
	if _, err := ResolveOVMFFirmware(); err == nil {
		t.Fatalf("expected error when only %s is set", ovmfCodeEnv)
	}

	t.Setenv(ovmfCodeEnv, code)
	t.Setenv(ovmfVarsEnv, vars)
	firmware, err := ResolveOVMFFirmware()
	if err != nil {
		t.Fatalf("ResolveOVMFFirmware failed: %v", err)
	}
	if firmware.CodePath != code || firmware.VarsTemplatePath != vars {
		t.Fatalf("firmware = %+v, want CODE=%s VARS=%s", firmware, code, vars)
	}
}

func TestResolveOVMFFirmwareRejectsUnreadableEnvPath(t *testing.T) {
	t.Setenv(ovmfCodeEnv, "/definitely/missing/OVMF_CODE.fd")
	t.Setenv(ovmfVarsEnv, "/definitely/missing/OVMF_VARS.fd")
	if _, err := ResolveOVMFFirmware(); err == nil {
		t.Fatal("expected missing env paths to fail")
	}
}

func TestResolveOVMFEnvOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	code, vars := writeTestOVMFFirmware(t, dir, "vars")

	tests := []struct {
		name    string
		code    string
		vars    string
		want    OVMFFirmware
		wantOK  bool
		wantErr bool
	}{
		{name: "none"},
		{
			name:    "code only",
			code:    code,
			wantOK:  true,
			wantErr: true,
		},
		{
			name:    "vars only",
			vars:    vars,
			wantOK:  true,
			wantErr: true,
		},
		{
			name:   "valid pair",
			code:   code,
			vars:   vars,
			want:   OVMFFirmware{CodePath: code, VarsTemplatePath: vars},
			wantOK: true,
		},
		{
			name:    "missing code",
			code:    filepath.Join(dir, "missing_CODE.fd"),
			vars:    vars,
			wantOK:  true,
			wantErr: true,
		},
		{
			name:    "missing vars",
			code:    code,
			vars:    filepath.Join(dir, "missing_VARS.fd"),
			wantOK:  true,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := resolveOVMFEnvOverride(tt.code, tt.vars)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("firmware = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOVMFEnvOverrideIncomplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		vars string
		want bool
	}{
		{name: "none"},
		{name: "code only", code: "/ovmf/code.fd", want: true},
		{name: "vars only", vars: "/ovmf/vars.fd", want: true},
		{name: "complete", code: "/ovmf/code.fd", vars: "/ovmf/vars.fd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ovmfEnvOverrideIncomplete(tt.code, tt.vars); got != tt.want {
				t.Fatalf("ovmfEnvOverrideIncomplete(%q, %q) = %v, want %v", tt.code, tt.vars, got, tt.want)
			}
		})
	}
}

func TestReadableOVMFPairRequiresBothFiles(t *testing.T) {
	dir := t.TempDir()
	code, vars := writeTestOVMFFirmware(t, dir, "vars")

	if !readableOVMFPair(code, vars) {
		t.Fatal("readableOVMFPair with existing CODE and VARS = false, want true")
	}
	if readableOVMFPair(code, filepath.Join(dir, "missing_VARS.fd")) {
		t.Fatal("readableOVMFPair with missing VARS = true, want false")
	}
}

func TestPrepareUEFIUsesInstanceVarsPath(t *testing.T) {
	dir := t.TempDir()
	code, varsTemplate := writeTestOVMFFirmware(t, dir, "vars-template")
	t.Setenv(ovmfCodeEnv, code)
	t.Setenv(ovmfVarsEnv, varsTemplate)

	workDir := filepath.Join(dir, "instance")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{}
	gotCode, gotVars, err := manager.prepareUEFI(workDir)
	if err != nil {
		t.Fatalf("prepareUEFI: %v", err)
	}
	if gotCode != code {
		t.Fatalf("code path = %q, want %q", gotCode, code)
	}
	if want := filepath.Join(workDir, "OVMF_VARS.fd"); gotVars != want {
		t.Fatalf("vars path = %q, want %q", gotVars, want)
	}
	data, err := os.ReadFile(gotVars)
	if err != nil {
		t.Fatalf("read copied vars: %v", err)
	}
	if string(data) != "vars-template" {
		t.Fatalf("copied vars = %q, want %q", string(data), "vars-template")
	}
	assertMode(t, gotVars, ovmfVarsPerm)
}
