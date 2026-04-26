package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOVMFFirmwareRequiresEnvPair(t *testing.T) {
	dir := t.TempDir()
	code := filepath.Join(dir, "OVMF_CODE.fd")
	vars := filepath.Join(dir, "OVMF_VARS.fd")
	if err := os.WriteFile(code, []byte("code"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vars, []byte("vars"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOLOS_OVMF_CODE", code)
	t.Setenv("HOLOS_OVMF_VARS", "")
	if _, err := ResolveOVMFFirmware(); err == nil {
		t.Fatal("expected error when only HOLOS_OVMF_CODE is set")
	}

	t.Setenv("HOLOS_OVMF_CODE", code)
	t.Setenv("HOLOS_OVMF_VARS", vars)
	firmware, err := ResolveOVMFFirmware()
	if err != nil {
		t.Fatalf("ResolveOVMFFirmware failed: %v", err)
	}
	if firmware.CodePath != code || firmware.VarsTemplatePath != vars {
		t.Fatalf("firmware = %+v, want CODE=%s VARS=%s", firmware, code, vars)
	}
}

func TestResolveOVMFFirmwareRejectsUnreadableEnvPath(t *testing.T) {
	t.Setenv("HOLOS_OVMF_CODE", "/definitely/missing/OVMF_CODE.fd")
	t.Setenv("HOLOS_OVMF_VARS", "/definitely/missing/OVMF_VARS.fd")
	if _, err := ResolveOVMFFirmware(); err == nil {
		t.Fatal("expected missing env paths to fail")
	}
}
