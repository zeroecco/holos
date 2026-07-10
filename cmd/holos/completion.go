package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// runCompletion prints self-contained completion scripts so installation does
// not require a package manager or an additional runtime dependency.
func runCompletion(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: holos completion <bash|zsh|fish>")
	}
	script, ok := completionScripts[args[0]]
	if !ok {
		return fmt.Errorf("unsupported shell %q (choose bash, zsh, or fish)", args[0])
	}
	_, err := fmt.Fprint(os.Stdout, script)
	return err
}

var completionCommands = func() string {
	names := append([]string(nil), commandOrder...)
	sort.Strings(names)
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	return strings.Join(quoted, " ")
}()

var completionScripts = map[string]string{
	"bash": fmt.Sprintf(`_holos_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands=(%s)
    if (( COMP_CWORD == 1 )); then
        COMPREPLY=($(compgen -W "${commands[*]}" -- "$cur"))
        return
    fi
    case "${COMP_WORDS[1]}" in
        snapshots) COMPREPLY=($(compgen -W "create list rm" -- "$cur"));;
        volumes) COMPREPLY=($(compgen -W "list rm remove export snapshot snapshots snapshot-rm resize" -- "$cur"));;
        completion) COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur"));;
    esac
}
complete -F _holos_completions holos
`, completionCommands),
	"zsh": fmt.Sprintf(`#compdef holos

_holos() {
    local -a commands
    commands=(%s)
    _describe 'holos command' commands
}
compdef _holos holos
`, completionCommands),
	"fish": fmt.Sprintf(`set -l holos_commands %s
complete -c holos -f -n '__fish_use_subcommand' -a "$holos_commands"
complete -c holos -f -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
complete -c holos -f -n '__fish_seen_subcommand_from snapshots' -a 'create list rm'
complete -c holos -f -n '__fish_seen_subcommand_from volumes' -a 'rm remove export snapshot snapshots snapshot-rm resize'
`, completionCommands),
}
