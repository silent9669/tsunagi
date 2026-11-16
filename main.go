// tsunagi runs Claude Code against Claude and other providers' models from
// one session, using your own keys.
package main

import (
	"fmt"
	"os"

	"github.com/silent9669/tsunagi/internal/ui"
)

const version = "0.1.0"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	t := ui.Detect()
	if len(args) > 0 {
		switch args[0] {
		case "login":
			return cmdLogin(t, args[1:])
		case "logout":
			return cmdLogout(t, args[1:])
		case "providers":
			return cmdProviders(t, args[1:])
		case "sync":
			return cmdSync(t)
		case "models":
			return cmdModels(t, args[1:])
		case "ping":
			return cmdPing(t, args[1:])
		case "import":
			return cmdImport(t, args[1:])
		case "watch":
			return cmdWatch(t)
		case "version":
			t.Printf("tsunagi %s\n", version)
			return 0
		case "help", "-h", "--help":
			return cmdHelp(t)
		}
	}
	return launch(t, args)
}

func cmdHelp(t *ui.Term) int {
	t.Splash()
	rows := [][2]string{
		{"tsunagi [claude args]", "launch claude through the router"},
		{"tsunagi login <provider>", "store a key in the macOS keychain"},
		{"tsunagi login chatgpt", "sign in with your ChatGPT plan"},
		{"tsunagi logout <provider>", "remove a key"},
		{"tsunagi providers [--all]", "list providers and key status"},
		{"tsunagi import opencode", "take providers and keys from opencode"},
		{"tsunagi sync", "refresh the provider catalogue"},
		{"tsunagi models <provider>", "list a provider's model names"},
		{"tsunagi ping [--tool] <model>", "send one test request through the router"},
		{"tsunagi watch", "live traffic from a running session"},
	}
	for _, r := range rows {
		t.Printf("  %s %s\n", t.Paint(ui.Mint, fmt.Sprintf("%-27s", r[0])), t.Dim(r[1]))
	}
	return 0
}
