package main

import (
	"slices"
	"testing"

	"github.com/zeroecco/holos/internal/runtime"
)

const (
	testExecIdentityPath = "/state/demo/ssh/id_ed25519"
	testExecLoginUser    = "debian"
	testExecSSHPort      = 2222
)

func TestBuildSSHArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		identity  string
		port      int
		user      string
		command   []string
		want      []string
		wantNoTTY bool
	}{
		{
			name:     "interactive",
			identity: testExecIdentityPath,
			port:     testExecSSHPort,
			user:     defaultExecLoginUser,
			want: []string{
				"-i", testExecIdentityPath,
				"-p", "2222",
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"-t",
				"ubuntu@127.0.0.1",
			},
		},
		{
			name:      "command",
			identity:  "/key",
			port:      2022,
			user:      testExecLoginUser,
			command:   []string{"echo", "hello"},
			wantNoTTY: true,
			want: []string{
				"-i", "/key",
				"-p", "2022",
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				"debian@127.0.0.1",
				"echo", "hello",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildSSHArgs(tt.identity, tt.port, tt.user, tt.command)
			assertStringSliceEqual(t, "buildSSHArgs", got, tt.want)
			if tt.wantNoTTY && slices.Contains(got, "-t") {
				t.Fatalf("buildSSHArgs includes %q: %v", "-t", got)
			}
		})
	}
}

func TestExecLoginUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		override   string
		targetUser string
		want       string
	}{
		{name: "override wins", override: "root", targetUser: testExecLoginUser, want: "root"},
		{name: "target user", targetUser: testExecLoginUser, want: testExecLoginUser},
		{name: "default", want: defaultExecLoginUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := execLoginUser(tt.override, tt.targetUser); got != tt.want {
				t.Fatalf("execLoginUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstanceIsRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		inst runtime.InstanceRecord
		want bool
	}{
		{name: "running", inst: runtime.InstanceRecord{Status: runtime.InstanceStatusRunning}, want: true},
		{name: "stopped", inst: runtime.InstanceRecord{Status: runtime.InstanceStatusStopped}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := instanceIsRunning(tt.inst); got != tt.want {
				t.Fatalf("instanceIsRunning = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstanceTargetRequiredError(t *testing.T) {
	t.Parallel()

	err := instanceTargetRequiredError("exec")
	if got, want := err.Error(), `exec requires a project name or an instance name (e.g. "my-stack" or "web-0")`; got != want {
		t.Fatalf("instanceTargetRequiredError = %q, want %q", got, want)
	}
}

func TestInstanceNotRunningError(t *testing.T) {
	t.Parallel()

	err := instanceNotRunningError(runtime.InstanceRecord{Name: "web-0", Status: runtime.InstanceStatusStopped})
	if got, want := err.Error(), `instance "web-0" is stopped`; got != want {
		t.Fatalf("instanceNotRunningError = %q, want %q", got, want)
	}
}

func TestInstanceFeatureSupport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		support func(runtime.InstanceRecord) bool
		err     func(runtime.InstanceRecord) error
		yes     runtime.InstanceRecord
		no      runtime.InstanceRecord
		wantErr string
	}{
		{
			name:    "exec",
			support: instanceSupportsExec,
			err:     instanceMissingExecSupportError,
			yes:     runtime.InstanceRecord{SSHPort: testExecSSHPort},
			no:      runtime.InstanceRecord{Name: "web-0"},
			wantErr: `instance "web-0" has no ssh port (created before exec support; recreate the stack)`,
		},
		{
			name:    "console",
			support: instanceSupportsConsole,
			err:     instanceMissingConsoleSupportError,
			yes:     runtime.InstanceRecord{SerialPath: "/state/demo/web-0/console.sock"},
			no:      runtime.InstanceRecord{Name: "web-0"},
			wantErr: `instance "web-0" has no serial console (created before console support)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.support(tt.yes) {
				t.Fatalf("%s support = false, want true for %+v", tt.name, tt.yes)
			}
			if tt.support(tt.no) {
				t.Fatalf("%s support = true, want false for %+v", tt.name, tt.no)
			}
			if got := tt.err(tt.no).Error(); got != tt.wantErr {
				t.Fatalf("%s missing support error = %q, want %q", tt.name, got, tt.wantErr)
			}
		})
	}
}
