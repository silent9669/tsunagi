package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/keychain"
	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/ui"
)

// keychainDelete removes one keychain entry; a package var so tests never
// touch the real keychain.
var keychainDelete = keychain.Delete

// uninstallPlan is everything `tsunagi uninstall` would remove.
type uninstallPlan struct {
	lines  []string // exact paths shown to the user
	skills bool
	cache  bool
	keys   bool
	config bool
}

// cmdUninstall removes what tsunagi put on the machine: installed skills,
// the cache dir, optionally keys and the config dir. It never removes the
// binary or touches ~/.claude/settings.json.
func cmdUninstall(t *ui.Term, args []string) int {
	var keys, cfg, yes bool
	var rest []string
	for _, a := range args {
		switch a {
		case "--keys":
			keys = true
		case "--config":
			cfg = true
		case "--yes", "-y":
			yes = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 {
		return fail(t, "usage: tsunagi uninstall [--keys] [--config] [--yes]")
	}
	plan := buildUninstallPlan(keys, cfg)
	printUninstallPlan(t, plan)
	if !yes {
		if !t.TTY {
			return 2
		}
		t.Printf("  remove everything? [y/N] ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			return 1
		}
	}
	runUninstall(plan)
	return 0
}

// buildUninstallPlan lists exactly what would be removed, without touching
// anything.
func buildUninstallPlan(keys, cfg bool) uninstallPlan {
	var p uninstallPlan
	p.skills = true // always: skills come out via the manifest
	p.keys = keys
	p.config = cfg

	// installed or outdated skills (modified/foreign files are left alone)
	for _, s := range skills.Status() {
		if s.State == "installed" || s.State == "outdated" {
			for _, tg := range skills.Targets() {
				if tg.ID == s.Target {
					p.lines = append(p.lines, filepath.Join(tg.Dir, s.Skill, "SKILL.md"))
				}
			}
		}
	}
	// the whole tsunagi cache subdir
	if base, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(base, "tsunagi")
		if _, err := os.Stat(dir); err == nil {
			p.cache = true
			p.lines = append(p.lines, dir)
		}
	}
	// keys: keychain entries plus chatgpt sign-in files (the config dir
	// listing below already covers the files when --config is set)
	if keys {
		names := config.Keyed()
		if len(names) > 0 {
			p.lines = append(p.lines, fmt.Sprintf("%d keychain entries", len(names)))
		}
		if !cfg {
			if dir := configDir(); dir != "" {
				matches, _ := filepath.Glob(filepath.Join(dir, "chatgpt*.json"))
				p.lines = append(p.lines, matches...)
			}
		}
	}
	// the config dir covers config.json, keys.json, chatgpt files, skills.json
	if cfg {
		if dir := configDir(); dir != "" {
			if _, err := os.Stat(dir); err == nil {
				p.lines = append(p.lines, dir)
			}
		}
	}
	return p
}

func configDir() string {
	p := config.Path()
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}

// printUninstallPlan shows the removal list and the binary hint.
func printUninstallPlan(t *ui.Term, p uninstallPlan) {
	for _, l := range p.lines {
		t.Printf("  %s\n", l)
	}
	if len(p.lines) == 0 {
		t.Printf("  nothing to remove\n")
	}
	if exe, err := os.Executable(); err == nil {
		t.Printf("  binary stays: brew uninstall tsunagi  or  rm %s\n", exe)
	}
}

// runUninstall performs the removals in plan.
func runUninstall(p uninstallPlan) {
	if p.skills {
		skills.Remove([]string{"claude", "agents"})
	}
	if p.cache {
		if base, err := os.UserCacheDir(); err == nil {
			os.RemoveAll(filepath.Join(base, "tsunagi"))
		}
	}
	if p.keys {
		for _, name := range config.Keyed() {
			keychainDelete(name)
		}
		if dir := configDir(); dir != "" {
			matches, _ := filepath.Glob(filepath.Join(dir, "chatgpt*.json"))
			for _, f := range matches {
				os.Remove(f)
			}
		}
	}
	if p.config {
		if dir := configDir(); dir != "" {
			os.RemoveAll(dir)
		}
	}
}
