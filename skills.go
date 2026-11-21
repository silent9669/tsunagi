package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/silent9669/tsunagi/internal/skills"
	"github.com/silent9669/tsunagi/internal/ui"
)

// cmdSkills shows skill install status, or installs/removes the skills into
// the agent skill directories.
func cmdSkills(t *ui.Term, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "install", "remove":
			return skillsChange(t, args[0], args[1:])
		case "-h", "--help":
			return cmdHelp(t, []string{"skills"})
		default:
			return skillsStatus(t, args)
		}
	}
	return skillsStatus(t, nil)
}

// skillsStatus prints the per-target x skill state table, or JSON.
func skillsStatus(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	if len(args) > 0 {
		return fail(t, "usage: tsunagi skills [--json]")
	}
	st := skills.Status()
	if asJSON {
		b, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return fail(t, "%v", err)
		}
		fmt.Println(string(b))
		return 0
	}
	targets := skills.Targets()
	// rows: one per skill, columns: one per target
	bySkill := map[string]map[string]string{}
	var names []string
	for _, s := range st {
		if _, ok := bySkill[s.Skill]; !ok {
			bySkill[s.Skill] = map[string]string{}
			names = append(names, s.Skill)
		}
		bySkill[s.Skill][s.Target] = s.State
	}
	headers := []string{"skill"}
	for _, tg := range targets {
		headers = append(headers, tg.ID)
	}
	var rows [][]string
	for _, n := range names {
		row := []string{n}
		for _, tg := range targets {
			row = append(row, bySkill[n][tg.ID])
		}
		rows = append(rows, row)
	}
	t.Table(headers, rows)
	return 0
}

// skillsChange runs install or remove for the chosen targets.
func skillsChange(t *ui.Term, verb string, args []string) int {
	var targetFlag string
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--target" {
			if i+1 >= len(args) {
				return fail(t, "flag --target needs a value")
			}
			i++
			targetFlag = args[i]
		} else {
			rest = append(rest, args[i])
		}
	}
	if len(rest) > 0 {
		return fail(t, "usage: tsunagi skills %s [--target claude|agents|all]", verb)
	}
	ids, ok := pickTargets(t, targetFlag)
	if !ok {
		return 1
	}
	var results []skills.Result
	if verb == "install" {
		results = skills.Install(ids, version)
	} else {
		results = skills.Remove(ids)
	}
	for _, r := range results {
		line := fmt.Sprintf("%s/%s  %s", r.Target, r.Skill, r.State)
		if r.Reason != "" {
			line += " (" + r.Reason + ")"
		}
		t.Printf("  %s\n", line)
	}
	return 0
}

// pickTargets resolves --target or asks on a TTY. ok is false on failure.
func pickTargets(t *ui.Term, flag string) ([]string, bool) {
	if flag != "" {
		switch flag {
		case "claude", "agents":
			return []string{flag}, true
		case "all":
			return []string{"claude", "agents"}, true
		}
		fmt.Fprintf(t.Out, "  unknown target %q (want claude, agents or all)\n", flag)
		return nil, false
	}
	if !t.TTY {
		fmt.Fprintf(t.Out, "  usage: tsunagi skills install|remove --target claude|agents|all\n")
		return nil, false
	}
	return askTargets(t)
}

// askTargets shows the numbered target prompt and reads one line.
func askTargets(t *ui.Term) ([]string, bool) {
	targets := skills.Targets()
	for i, tg := range targets {
		t.Printf("  %d  %s\n", i+1, tg.Name)
	}
	t.Printf("  3  both\n  choice [1-3]: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return nil, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > 3 {
		t.Printf("  pick 1, 2 or 3\n")
		return nil, false
	}
	switch n {
	case 1:
		return []string{targets[0].ID}, true
	case 2:
		return []string{targets[1].ID}, true
	}
	return []string{"claude", "agents"}, true
}
