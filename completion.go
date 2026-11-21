package main

import (
	"fmt"
	"strings"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/ui"
)

func cmdCompletion(t *ui.Term, args []string) int {
	if len(args) != 1 {
		return fail(t, "usage: tsunagi completion zsh|bash|fish")
	}
	var names []string
	for _, c := range commandList() {
		names = append(names, c.name)
	}
	cmds := strings.Join(names, " ")
	switch args[0] {
	case "bash":
		fmt.Printf(`_tsunagi() {
  local cur=${COMP_WORDS[COMP_CWORD]} prev=${COMP_WORDS[COMP_CWORD-1]}
  if [ "$COMP_CWORD" -eq 1 ]; then COMPREPLY=($(compgen -W "%s --plain --no-color --no-anim" -- "$cur")); return; fi
  case "$prev" in
    login|logout|models|remove) COMPREPLY=($(compgen -W "$(tsunagi providers --names 2>/dev/null)" -- "$cur"));;
    *) COMPREPLY=($(compgen -W "--json --all --live --as --tool --tokens --session --timeout --days --log" -- "$cur"));;
  esac
}
complete -F _tsunagi tsunagi
`, cmds)
	case "zsh":
		fmt.Printf(`#compdef tsunagi
_tsunagi() {
  local -a cmds
  cmds=(%s)
  if (( CURRENT == 2 )); then _describe 'command' cmds; return; fi
  case $words[2] in
    login|logout|models|remove) compadd -- ${(f)"$(tsunagi providers --names 2>/dev/null)"};;
    *) compadd -- --json --all --live --as --tool --tokens --session --timeout --days --log;;
  esac
}
_tsunagi "$@"
`, zshCmds())
	case "fish":
		for _, c := range commandList() {
			fmt.Printf("complete -c tsunagi -n '__fish_use_subcommand' -a %s -d '%s'\n", c.name, strings.ReplaceAll(c.summary, "'", ""))
		}
		fmt.Printf("complete -c tsunagi -n '__fish_seen_subcommand_from login logout models remove' -a '(tsunagi providers --names 2>/dev/null)'\n")
	default:
		return fail(t, "usage: tsunagi completion zsh|bash|fish")
	}
	return 0
}

func zshCmds() string {
	var out []string
	for _, c := range commandList() {
		out = append(out, fmt.Sprintf("'%s:%s'", c.name, strings.ReplaceAll(c.summary, "'", "")))
	}
	return strings.Join(out, " ")
}

// providerNames prints provider names one per line, for shell completion.
func printProviderNames(t *ui.Term) int {
	cfg, err := config.Load()
	if err != nil {
		return 1
	}
	for _, p := range cfg.Providers {
		t.Printf("%s\n", p.Name)
	}
	return 0
}
