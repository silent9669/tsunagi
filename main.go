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
			return cmdProviders(t)
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
		{"tsunagi logout <provider>", "remove a key"},
		{"tsunagi providers", "list providers and key status"},
		{"tsunagi watch", "live traffic from a running session"},
	}
	for _, r := range rows {
		t.Printf("  %s %s\n", t.Paint(ui.Mint, fmt.Sprintf("%-27s", r[0])), t.Dim(r[1]))
	}
	return 0
}
