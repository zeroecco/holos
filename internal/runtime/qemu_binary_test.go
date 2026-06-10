package runtime

import "testing"

func TestQEMUBinaryEnvironmentOverrides(t *testing.T) {
	manager := NewManager(t.TempDir())

	t.Setenv(qemuSystemEnv, "/custom/qemu-system")
	if got, err := manager.qemuSystemBinary(); err != nil || got != "/custom/qemu-system" {
		t.Fatalf("qemuSystemBinary() = (%q, %v), want override", got, err)
	}

	t.Setenv(qemuImgEnv, "/custom/qemu-img")
	if got, err := manager.qemuImgBinary(); err != nil || got != "/custom/qemu-img" {
		t.Fatalf("qemuImgBinary() = (%q, %v), want override", got, err)
	}
}

func TestBinaryFromEnvOrPathUsesOverride(t *testing.T) {
	const envName = "HOLOS_TEST_BINARY_OVERRIDE"
	t.Setenv(envName, "/custom/tool")

	got, err := binaryFromEnvOrPath(envName, "definitely-missing-tool", "install test tool")
	if err != nil {
		t.Fatalf("binaryFromEnvOrPath: %v", err)
	}
	if got != "/custom/tool" {
		t.Fatalf("binaryFromEnvOrPath = %q, want override", got)
	}
}

func TestBinaryFromEnvOrPathMissingBinaryError(t *testing.T) {
	t.Parallel()

	got, err := binaryFromEnvOrPath("HOLOS_TEST_MISSING_BINARY", "definitely-missing-tool", "install test tool")
	if err == nil {
		t.Fatalf("binaryFromEnvOrPath = %q, want error", got)
	}
	assertErrorContains(t, err, "definitely-missing-tool not found", "install test tool", "HOLOS_TEST_MISSING_BINARY")
}

func TestMissingBinaryError(t *testing.T) {
	t.Parallel()

	err := missingBinaryError("HOLOS_TEST_BINARY", "missing-tool", "install test package")
	assertErrorContains(t, err, "missing-tool not found", "install test package", "HOLOS_TEST_BINARY")
}
