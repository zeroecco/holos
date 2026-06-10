package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQEMUStartupProbeDelay(t *testing.T) {
	t.Parallel()

	if qemuStartupProbeDelay <= 0 {
		t.Fatalf("qemuStartupProbeDelay = %s, want positive", qemuStartupProbeDelay)
	}
	if qemuStartupProbeDelay > time.Second {
		t.Fatalf("qemuStartupProbeDelay = %s, want <= 1s", qemuStartupProbeDelay)
	}
}

func TestLaunchQEMUCreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	qemuSystem := filepath.Join(dir, qemuSystemDefault)
	qemuOutput := "qemu-mock"
	script := "#!/bin/sh\necho " + qemuOutput + "\nexit 1\n"
	if err := os.WriteFile(qemuSystem, []byte(script), 0o755); err != nil {
		t.Fatalf("write qemu-system mock: %v", err)
	}
	t.Setenv(qemuSystemEnv, qemuSystem)

	logPath := filepath.Join(dir, instanceQEMULogFilename)
	manager := &Manager{}
	_, logText, err := manager.launchQEMU(nil, logPath, instanceDirName("web", 0))
	if err == nil {
		t.Fatal("launchQEMU succeeded for early-exiting qemu mock")
	}
	if logText != qemuOutput {
		t.Fatalf("logText = %q, want %q", logText, qemuOutput)
	}
	assertMode(t, logPath, qemuLogPerm)
}

func TestIsQEMUHostPortConflict(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want bool
	}{
		{
			name: "address already in use with hostfwd",
			log:  "qemu-system-x86_64: -netdev user,id=net0,hostfwd=tcp:127.0.0.1:8080-:80: address already in use",
			want: true,
		},
		{
			name: "could not set up host forwarding",
			log:  "hostfwd=tcp::2222-:22: Could not set up host forwarding rule",
			want: true,
		},
		{
			name: "failed to set up host forwarding",
			log:  "HOSTFWD tcp::2222-:22 failed to set up host forwarding",
			want: true,
		},
		{
			name: "address conflict without hostfwd context",
			log:  "listen tcp 127.0.0.1:8080: address already in use",
			want: false,
		},
		{
			name: "hostfwd unrelated failure",
			log:  "hostfwd=tcp::2222-:22: invalid host forwarding rule",
			want: false,
		},
		{
			name: "empty log",
			log:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isQEMUHostPortConflict(tt.log); got != tt.want {
				t.Fatalf("isQEMUHostPortConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}
