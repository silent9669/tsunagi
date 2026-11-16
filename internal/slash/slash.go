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
	run  string // fixed arguments after the executable; never user input
}

// The commands take no arguments. User text spliced into a shell line would be
// an injection point, and the exact command line is what the tool grant allows.
var commands = []command{
	{"usage", "Tokens per account and live balances", "usage"},
	{"accounts", "List every key and sign-in", "accounts"},
	{"providers", "List providers and key status", "providers"},
	{"models", "A few model names per ready provider", "models --summary"},
	{"sessions", "Running tsunagi sessions", "sessions"},
	{"handoff", "How to pass a plan to another tsunagi session", "handoff"},
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
		// only the user may run these, and only this exact command line
		b.WriteString("disable-model-invocation: true\n")
		fmt.Fprintf(&b, "allowed-tools: Bash(%s %s)\n", exe, c.run)
		b.WriteString("---\n")
		fmt.Fprintf(&b, "Output of `tsunagi %s`:\n\n```\n!`%s %s`\n```\n\n", c.run, exe, c.run)
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
