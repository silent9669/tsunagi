// Package slash writes a small Claude Code plugin that adds tsunagi slash
// commands (/tsunagi:usage, /tsunagi:models, ...) to a launched session. The
// launcher passes it with --plugin-dir, so nothing is installed in the user's
// own Claude Code configuration.
package slash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type command struct {
	name string
	desc string
	hint string // argument hint, "" for none
	run  string // what to run, after the executable; may use $ARGUMENTS
}

var commands = []command{
	{"usage", "Tokens per account and live balances", "[--days N]", "usage $ARGUMENTS"},
	{"accounts", "List every key and sign-in", "", "accounts"},
	{"providers", "List providers and key status", "[--all]", "providers $ARGUMENTS"},
	{"models", "List the model names to type after /model", "<provider>", "models $ARGUMENTS"},
	{"ping", "Send one test request through the router", "[--tool] <provider/model>", "ping $ARGUMENTS"},
	{"sessions", "Running tsunagi sessions", "", "sessions"},
	{"handoff", "How to pass a plan to another tsunagi session", "[--session id]", "handoff $ARGUMENTS"},
}

// safeExe accepts paths that need no quoting in a command line.
var safeExe = regexp.MustCompile(`^/[A-Za-z0-9_./+-]+$`)

// Write creates the plugin in dir. exe is the tsunagi binary the commands run.
func Write(dir, exe string) error {
	if !safeExe.MatchString(exe) {
		return fmt.Errorf("the tsunagi path %q has characters that need quoting; slash commands are skipped", exe)
	}
	manifest, _ := json.Marshal(map[string]string{
		"name": "tsunagi", "version": "1",
		"description": "tsunagi: routing, accounts and usage from inside Claude Code",
	})
	files := map[string]string{filepath.Join(".claude-plugin", "plugin.json"): string(manifest)}
	for _, c := range commands {
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "description: %s\n", c.desc)
		if c.hint != "" {
			fmt.Fprintf(&b, "argument-hint: %s\n", c.hint)
		}
		fmt.Fprintf(&b, "allowed-tools: Bash(%s:*)\n", exe)
		b.WriteString("---\n")
		fmt.Fprintf(&b, "Output of `tsunagi %s`:\n\n```\n!`%s %s`\n```\n\n", strings.Fields(c.run)[0], exe, c.run)
		b.WriteString("Reply with that output exactly, in a code block, and nothing else.\n")
		files[filepath.Join("commands", c.name+".md")] = b.String()
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Names lists the commands as the user types them.
func Names() []string {
	var n []string
	for _, c := range commands {
		n = append(n, "/tsunagi:"+c.name)
	}
	return n
}
