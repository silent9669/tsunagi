// tsunagi runs Claude Code against Claude and other providers' models from
// one session, using your own keys.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/silent9669/tsunagi/internal/ui"
)

const version = "0.2.0"

func main() { os.Exit(run(os.Args[1:])) }

type command struct {
	name    string
	group   string
	summary string
	usage   string
	run     func(t *ui.Term, args []string) int
}

func commandList() []command {
	return []command{
		{"login", "Accounts", "store a key, or sign in with ChatGPT", "tsunagi login <provider> [--as <label>]\n  tsunagi login chatgpt [--as <label>] [--new]", cmdLogin},
		{"logout", "Accounts", "remove a key or sign-in", "tsunagi logout <provider> [--as <label>]", cmdLogout},
		{"accounts", "Accounts", "list every key and sign-in", "tsunagi accounts [--json]", func(t *ui.Term, a []string) int { return cmdAccounts(t, a) }},
		{"usage", "Accounts", "traffic per account, plus balances", "tsunagi usage [--days N] [--json]", cmdUsage},
		{"providers", "Providers", "list providers and key status", "tsunagi providers [--all] [--json]", cmdProviders},
		{"models", "Providers", "list a provider's model names", "tsunagi models [--live] [--json] <provider>", cmdModels},
		{"add", "Providers", "add your own gateway or provider", "tsunagi add <name> <url> [--dialect d] [--local] [--header \"K: V\"] [--auth-header h] [--strategy s]", cmdAdd},
		{"remove", "Providers", "remove a provider you added", "tsunagi remove <name>", cmdRemove},
		{"sync", "Providers", "refresh the models.dev catalogue", "tsunagi sync", func(t *ui.Term, _ []string) int { return cmdSync(t) }},
		{"import", "Providers", "take providers and keys from opencode", "tsunagi import opencode", cmdImport},
		{"free", "Providers", "free-model offers, probes and the free/auto combo", "tsunagi free [--all] [--json] [provider]\n  tsunagi free refresh | probe [provider] | combo [--no-probe] [--dry-run] | allow|deny <provider>", cmdFree},
		{"ping", "Check", "send one test request through the router", "tsunagi ping [--tool] <provider/model>", cmdPing},
		{"route", "Check", "show how a model name would be routed", "tsunagi route [--tools] [--tokens N] [--json] <model>", cmdRoute},
		{"doctor", "Check", "check that everything is in place", "tsunagi doctor", func(t *ui.Term, _ []string) int { return cmdDoctor(t) }},
		{"watch", "Sessions", "live traffic from every session", "tsunagi watch [--log | --json]", cmdWatch},
		{"sessions", "Sessions", "running sessions, and when each last finished", "tsunagi sessions [--json]", cmdSessions},
		{"handoff", "Sessions", "how to pass a plan to a session", "tsunagi handoff [--session <id>]", cmdHandoff},
		{"wait", "Sessions", "block until a session finishes a turn", "tsunagi wait [--session <id>] [--timeout 30m]", cmdWait},
		{"done", "Sessions", "record that this session finished", "tsunagi done <summary>", cmdDone},
		{"statusline", "Info", "one-line status for Claude Code's status line", "tsunagi statusline   (reads Claude Code's JSON on stdin)", cmdStatusline},
		{"completion", "Info", "shell completion script", "tsunagi completion zsh|bash|fish", cmdCompletion},
		{"version", "Info", "print the version", "tsunagi version", func(t *ui.Term, _ []string) int { t.Printf("tsunagi %s\n", version); return 0 }},
		{"help", "Info", "show help, for one command with: help <command>", "tsunagi help [command]", cmdHelp},
	}
}

// globalFlags reads display flags that come before the command. They never
// reach Claude Code, because they only count in front of a tsunagi command.
func globalFlags(t *ui.Term, args []string) []string {
	for len(args) > 0 {
		switch args[0] {
		case "--plain":
			t.Color, t.True, t.Anim = false, false, false
		case "--no-color":
			t.Color, t.True, t.Anim = false, false, false
		case "--no-anim":
			t.Anim = false
		default:
			return args
		}
		args = args[1:]
	}
	return args
}

func run(args []string) int {
	t := ui.Detect()
	args = globalFlags(t, args)
	if len(args) > 0 {
		name := args[0]
		if name == "-h" || name == "--help" {
			name = "help"
		}
		for _, c := range commandList() {
			if c.name != name {
				continue
			}
			rest := args[1:]
			if name != "help" && len(rest) == 1 && (rest[0] == "--help" || rest[0] == "-h") {
				return cmdHelp(t, []string{name})
			}
			if args[0] == "-h" || args[0] == "--help" {
				rest = nil
			}
			return c.run(t, rest)
		}
	}
	return launch(t, args)
}

func cmdHelp(t *ui.Term, args []string) int {
	cmds := commandList()
	if len(args) == 1 {
		for _, c := range cmds {
			if c.name == args[0] {
				t.Printf("  %s  %s\n\n  %s\n", t.Bold("tsunagi "+c.name), t.Dim(c.summary), c.usage)
				return 0
			}
		}
		return fail(t, "no command %q. Run: tsunagi help", args[0])
	}
	t.Splash()
	t.Printf("  %s\n\n", t.Dim("tsunagi [claude args] launches Claude Code through the router"))
	group := ""
	for _, c := range cmds {
		if c.group != group {
			if group != "" {
				t.Printf("\n")
			}
			group = c.group
			t.Printf("  %s\n", t.Bold(group))
		}
		t.Printf("    %s %s\n", t.Paint(ui.Mint, fmt.Sprintf("%-11s", c.name)), t.Dim(c.summary))
	}
	t.Printf("\n  %s\n", t.Dim("flags before a command: --plain  --no-color  --no-anim. Data commands take --json."))
	return 0
}

// popFlag removes name from args and reports whether it was there.
func popFlag(args []string, name string) ([]string, bool) {
	var out []string
	found := false
	for _, a := range args {
		if a == name {
			found = true
		} else {
			out = append(out, a)
		}
	}
	return out, found
}

func joinNames(cmds []command) string {
	var n []string
	for _, c := range cmds {
		n = append(n, c.name)
	}
	return strings.Join(n, " ")
}
