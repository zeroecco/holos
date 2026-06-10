package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	commandPartSeparator = " "
	commandChangeDir     = "cd "
	commandAndThen       = " && "
)

// ComposeCommand accepts Docker Compose command/entrypoint string, list, null,
// and empty forms.
type ComposeCommand struct {
	Args   []string
	Set    bool
	Scalar bool
}

func (c *ComposeCommand) UnmarshalYAML(node *yaml.Node) error {
	c.Set = true
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == yamlNullTag {
			c.Args = nil
			return nil
		}
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		if s == "" {
			c.Args = nil
		} else {
			c.Args = []string{s}
			c.Scalar = true
		}
		return nil
	case yaml.SequenceNode:
		var args []string
		if err := node.Decode(&args); err != nil {
			return err
		}
		c.Args = args
		return nil
	default:
		return fmt.Errorf("line %d: command values must be a string, list, or null", node.Line)
	}
}

func composeRunCmd(entrypoint, command ComposeCommand, workingDir string) []string {
	parts := composeCommandFragments(entrypoint, command)
	if len(parts) == 0 {
		return nil
	}
	cmd := commandWithWorkingDir(strings.Join(parts, commandPartSeparator), workingDir)
	return []string{cmd}
}

func composeCommandFragments(entrypoint, command ComposeCommand) []string {
	parts := make([]string, 0, len(entrypoint.Args)+len(command.Args))
	if len(entrypoint.Args) > 0 {
		parts = append(parts, entrypoint.shellFragment())
	}
	if len(command.Args) > 0 {
		parts = append(parts, command.shellFragment())
	}
	return parts
}

func commandWithWorkingDir(cmd, workingDir string) string {
	if workingDir == "" {
		return cmd
	}
	return commandChangeDir + shellQuote(workingDir) + commandAndThen + cmd
}

func serviceRunCmd(svc Service, dfRunCmd []string) []string {
	out := append([]string{}, dfRunCmd...)
	out = append(out, composeRunCmd(svc.Entrypoint, svc.Command, svc.WorkingDir)...)
	out = append(out, lifecycleRunCmd(svc.PostStart)...)
	out = append(out, lifecycleRunCmd(svc.PreStop)...)
	out = append(out, svc.CloudInit.RunCmd...)
	return out
}

func lifecycleRunCmd(hooks []LifecycleHook) []string {
	var out []string
	for _, hook := range hooks {
		out = append(out, lifecycleHookRunCmd(hook)...)
	}
	return out
}

func lifecycleHookRunCmd(hook LifecycleHook) []string {
	cmds := composeRunCmd(ComposeCommand{}, hook.Command, hook.WorkingDir)
	env := environmentPrefix(hook.Environment)
	if env == "" {
		return cmds
	}
	return prefixCommands(env, cmds)
}

func prefixCommands(prefix string, cmds []string) []string {
	out := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		out = append(out, prefix+commandPartSeparator+cmd)
	}
	return out
}
