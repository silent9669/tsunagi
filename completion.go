package main

import (
	"fmt"
	"strings"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/ui"
)

// lookNames and themeNames feed flag-value completion for `tsunagi look`.
func lookNames() string {
	var n []string
	for _, l := range art.Looks {
		n = append(n, string(l))
	}
	return strings.Join(n, " ")
}

func themeNames() string { return strings.Join(tui.ThemeNames, " ") }

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
    --style|--look) COMPREPLY=($(compgen -W "`+lookNames()+`" -- "$cur"));;
    --theme) COMPREPLY=($(compgen -W "`+themeNames()+`" -- "$cur"));;
    *) COMPREPLY=($(compgen -W "--json --all --live --as --tool --tokens --session --timeout --days --log --dry-run --no-probe --look --theme --style --static" -- "$cur"));;
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
    --style|--look) compadd -- `+lookNames()+`;;
    --theme) compadd -- `+themeNames()+`;;
    *) compadd -- --json --all --live --as --tool --tokens --session --timeout --days --log --dry-run --no-probe --look --theme --style --static;;
  esac
}
_tsunagi "$@"
`, zshCmds())
	case "fish":
		for _, c := range commandList() {
			fmt.Printf("complete -c tsunagi -n '__fish_use_subcommand' -a %s -d '%s'\n", c.name, strings.ReplaceAll(c.summary, "'", ""))
		}
		fmt.Printf("complete -c tsunagi -n '__fish_seen_subcommand_from login logout models remove' -a '(tsunagi providers --names 2>/dev/null)'\n")
		fmt.Printf("complete -c tsunagi -n '__fish_seen_subcommand look' -l style -xa '%s'\n", lookNames())
		fmt.Printf("complete -c tsunagi -n '__fish_seen_subcommand look' -l theme -xa '%s'\n", themeNames())
		fmt.Printf("complete -c tsunagi -n '__fish_seen_subcommand look' -l static\n")
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
