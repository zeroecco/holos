package compose

import "testing"

const (
	testCommandWorkDir = "/srv/app"
	testEnvCommand     = "echo $HOME"
	testReadyCommand   = "echo ready"
)

func testHelloWorldCommand() []string {
	return []string{"echo", "hello world"}
}

func TestResolveAcceptsCommandAndEntrypointSyntax(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestImage(t, dir)
	yamlDoc := `
name: commands
services:
  string:
    image: ./base.qcow2
    command: echo hello
    entrypoint: /bin/sh -c
    working_dir: /srv/app
  list:
    image: ./base.qcow2
    command: ["echo", "hello world"]
    entrypoint: ["/usr/bin/env"]
  hooks:
    image: ./base.qcow2
    post_start:
      - command: echo started
        working_dir: /srv/app
        environment:
          HOOK: post
      - command: ["touch", "/tmp/ready"]
    pre_stop:
      - command: echo stopping
`
	project := resolveTestCompose(t, dir, yamlDoc)
	assertStringSliceEqual(t, "string runcmd", project.Services["string"].CloudInit.RunCmd, []string{"cd /srv/app && /bin/sh -c echo hello"})
	assertStringSliceEqual(t, "list runcmd", project.Services["list"].CloudInit.RunCmd, []string{"/usr/bin/env echo 'hello world'"})
	assertStringSliceEqual(t, "hooks runcmd", project.Services["hooks"].CloudInit.RunCmd, []string{
		"HOOK=post cd /srv/app && echo started",
		"touch /tmp/ready",
	})
	assertStringSliceEqual(t, "hooks pre_stop", project.Services["hooks"].PreStopCommands, []string{"echo stopping"})
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: "''"},
		{name: "safe path", input: testCommandWorkDir, want: testCommandWorkDir},
		{name: "space", input: "hello world", want: "'hello world'"},
		{name: "single quote", input: "it's", want: "'it'\"'\"'s'"},
		{name: "shell variable", input: "$HOME", want: "'$HOME'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shellQuote(tc.input); got != tc.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestComposeCommandShellFragment(t *testing.T) {
	t.Parallel()

	scalar := ComposeCommand{Args: []string{testEnvCommand}, Scalar: true}
	if got := scalar.shellFragment(); got != testEnvCommand {
		t.Fatalf("scalar shellFragment = %q", got)
	}

	list := ComposeCommand{Args: []string{"echo", "$HOME"}}
	if got := list.shellFragment(); got != "echo '$HOME'" {
		t.Fatalf("list shellFragment = %q", got)
	}
}

func TestComposeRunCmd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entrypoint ComposeCommand
		command    ComposeCommand
		workingDir string
		want       []string
	}{
		{name: "empty"},
		{
			name:       "entrypoint and list command",
			entrypoint: ComposeCommand{Args: []string{"/usr/bin/env"}},
			command:    ComposeCommand{Args: testHelloWorldCommand()},
			want:       []string{"/usr/bin/env echo 'hello world'"},
		},
		{
			name:       "working directory",
			command:    ComposeCommand{Args: []string{testEnvCommand}, Scalar: true},
			workingDir: testCommandWorkDir,
			want:       []string{"cd /srv/app && echo $HOME"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := composeRunCmd(tt.entrypoint, tt.command, tt.workingDir)
			assertStringSliceEqual(t, "composeRunCmd", got, tt.want)
		})
	}
}

func TestComposeCommandFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entrypoint ComposeCommand
		command    ComposeCommand
		want       []string
	}{
		{name: "empty"},
		{
			name:       "entrypoint only",
			entrypoint: ComposeCommand{Args: []string{"/usr/bin/env"}},
			want:       []string{"/usr/bin/env"},
		},
		{
			name:    "scalar command",
			command: ComposeCommand{Args: []string{testEnvCommand}, Scalar: true},
			want:    []string{"echo $HOME"},
		},
		{
			name:       "entrypoint and list command",
			entrypoint: ComposeCommand{Args: []string{"/usr/bin/env"}},
			command:    ComposeCommand{Args: testHelloWorldCommand()},
			want:       []string{"/usr/bin/env", "echo 'hello world'"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := composeCommandFragments(tt.entrypoint, tt.command)
			assertStringSliceEqual(t, "composeCommandFragments", got, tt.want)
		})
	}
}

func TestPrefixCommands(t *testing.T) {
	t.Parallel()

	got := prefixCommands("HOOK=post", []string{testReadyCommand, "touch /tmp/ready"})
	want := []string{"HOOK=post echo ready", "HOOK=post touch /tmp/ready"}
	assertStringSliceEqual(t, "prefixCommands", got, want)
}

func TestLifecycleHookRunCmd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hook LifecycleHook
		want []string
	}{
		{name: "empty"},
		{
			name: "command with working dir",
			hook: LifecycleHook{
				Command:    ComposeCommand{Args: []string{testReadyCommand}, Scalar: true},
				WorkingDir: testCommandWorkDir,
			},
			want: []string{"cd /srv/app && echo ready"},
		},
		{
			name: "environment prefix",
			hook: LifecycleHook{
				Command: ComposeCommand{Args: []string{testReadyCommand}, Scalar: true},
				Environment: Environment{
					"HOOK": stringPtr("post"),
				},
			},
			want: []string{"HOOK=post echo ready"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lifecycleHookRunCmd(tt.hook)
			assertStringSliceEqual(t, "lifecycleHookRunCmd", got, tt.want)
		})
	}
}
