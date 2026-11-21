package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/handoff"
	"github.com/silent9669/tsunagi/internal/slash"
	"github.com/silent9669/tsunagi/internal/ui"
)

// stopHookArgs returns args with a Stop hook added, so every finished turn
// writes the session's done file. It leaves args alone when the user passes
// their own --settings or sets TSUNAGI_NO_HOOK.
func stopHookArgs(args []string, exe string) []string {
	if os.Getenv("TSUNAGI_NO_HOOK") != "" {
		return args
	}
	for _, a := range args {
		if a == "--settings" || strings.HasPrefix(a, "--settings=") {
			return args
		}
	}
	stop := handoff.ShellQuote(exe) + " done --hook"
	status := ""
	if os.Getenv("TSUNAGI_STATUSLINE") != "" { // opt-in: it replaces the user's own status line
		status = handoff.ShellQuote(exe) + " statusline"
	}
	return append([]string{"--settings", handoff.Settings(stop, status)}, args...)
}

// pinArgs makes this session's `--settings` carry env values that must win
// over a lower-scope settings file (the router address over an env block that
// points Claude elsewhere). A `--settings` the user passed as a file is left
// alone: it cannot be merged.
func pinArgs(args []string, pins map[string]string) []string {
	if len(pins) == 0 {
		return args
	}
	for i, a := range args {
		if strings.HasPrefix(a, "--settings=") {
			return args
		}
		if a == "--settings" && i+1 < len(args) {
			merged, ok := handoff.MergeEnv(args[i+1], pins)
			if !ok {
				return args
			}
			out := append([]string(nil), args...)
			out[i+1] = merged
			return out
		}
	}
	return append([]string{"--settings", handoff.EnvSettings(pins)}, args...)
}

// registerSession records this launch for `tsunagi sessions` and returns the
// environment the child needs, plus the cleanup.
func registerSession() (env []string, cleanup func()) {
	base, err := events.Dir()
	if err != nil {
		return nil, func() {}
	}
	cwd, _ := os.Getwd()
	tty := ""
	c := exec.Command("tty")
	c.Stdin = os.Stdin
	if out, err := c.Output(); err == nil {
		tty = strings.TrimSpace(string(out))
	}
	pid := os.Getpid()
	cleanup, err = handoff.Register(base, handoff.Session{PID: pid, CWD: cwd, TTY: tty, Started: time.Now()})
	if err != nil {
		return nil, func() {}
	}
	return []string{"TSUNAGI_SESSION=" + strconv.Itoa(pid), "TSUNAGI_DONE=" + handoff.DonePath(base, pid)}, cleanup
}

// cmdDone writes the done file of the current session. As a Stop hook it reads
// Claude Code's JSON from stdin and uses the last assistant message as the
// summary; by hand it takes the summary as arguments, or from stdin with "-".
func cmdDone(t *ui.Term, args []string) int {
	path := os.Getenv("TSUNAGI_DONE")
	if len(args) > 0 && args[0] == "--hook" {
		if path == "" {
			return 0 // not a tsunagi session: stay out of the way
		}
		var in struct {
			TranscriptPath string `json:"transcript_path"`
		}
		b, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		json.Unmarshal(b, &in)
		summary := handoff.SettledAssistantText(in.TranscriptPath, 3*time.Second)
		if summary == "" {
			summary = "(the session finished without a text reply)"
		}
		handoff.WriteDone(path, handoff.Done{Session: sessionPID(), Source: "hook", Summary: summary})
		return 0 // a hook must never fail the session
	}
	if path == "" {
		return fail(t, "TSUNAGI_DONE is not set: run this inside a tsunagi session")
	}
	summary := strings.Join(args, " ")
	if summary == "-" {
		b, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		summary = string(b)
	}
	if strings.TrimSpace(summary) == "" {
		return fail(t, "usage: tsunagi done <summary>")
	}
	if err := handoff.WriteDone(path, handoff.Done{Session: sessionPID(), Source: "manual", Summary: strings.TrimSpace(summary)}); err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("  %s done recorded\n", t.Paint(ui.Mint, "[ok]"))
	return 0
}

func sessionPID() int {
	n, _ := strconv.Atoi(os.Getenv("TSUNAGI_SESSION"))
	return n
}

// pickSession chooses the session to hand a plan to: the one named, else the
// only other live one. The caller's own session (TSUNAGI_SESSION) is skipped.
func pickSession(list []handoff.Session, named int, self int) (handoff.Session, error) {
	var others []handoff.Session
	for _, s := range list {
		if s.PID == self {
			continue
		}
		if named != 0 && s.PID == named {
			return s, nil
		}
		others = append(others, s)
	}
	switch {
	case named != 0:
		return handoff.Session{}, fmt.Errorf("no live session %d. Run: tsunagi sessions", named)
	case len(others) == 1:
		return others[0], nil
	case len(others) == 0:
		return handoff.Session{}, fmt.Errorf("no other tsunagi session is running. Start one with: tsunagi")
	}
	return handoff.Session{}, fmt.Errorf("%d sessions are running; pick one with --session <id> (tsunagi sessions)", len(others))
}

func sessionFlag(args []string) (rest []string, id int, timeout time.Duration, err error) {
	timeout = 30 * time.Minute
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session", "--timeout":
			if i+1 >= len(args) {
				return nil, 0, 0, fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--session" {
				if id, err = strconv.Atoi(args[i+1]); err != nil {
					return nil, 0, 0, fmt.Errorf("--session takes a session id")
				}
			} else if timeout, err = time.ParseDuration(args[i+1]); err != nil {
				return nil, 0, 0, fmt.Errorf("--timeout takes a duration such as 20m")
			}
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	return rest, id, timeout, nil
}

func cmdSessions(t *ui.Term, args []string) int {
	args, asJSON := popFlag(args, "--json")
	if len(args) > 0 {
		return fail(t, "usage: tsunagi sessions [--json]")
	}
	base, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	list := handoff.List(base)
	if asJSON {
		data := make([]map[string]any, 0, len(list))
		for _, s := range list {
			row := map[string]any{"session": s.PID, "cwd": s.CWD, "tty": s.TTY, "started": s.Started, "done_file": s.Done}
			if d, ok := handoff.ReadDone(s.Done); ok {
				row["last_finished"] = d.Time
				row["summary"] = d.Summary
			}
			data = append(data, row)
		}
		return printJSON(t, data)
	}
	if len(list) == 0 {
		t.Printf("  %s\n", t.Dim("no tsunagi session is running"))
		return 0
	}
	var rows [][]string
	for _, s := range list {
		last := t.Dim("-")
		if d, ok := handoff.ReadDone(s.Done); ok {
			last = ago(d.Time) + " " + clip(oneLine(d.Summary), 40)
		}
		rows = append(rows, []string{strconv.Itoa(s.PID), clip(s.CWD, 36), s.TTY, ago(s.Started), last})
	}
	t.Table([]string{"session", "cwd", "tty", "started", "last finished"}, rows)
	return 0
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func ago(at time.Time) string {
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

// cmdWait blocks until the session finishes a turn, then prints its summary.
// A Claude session runs it in the background and is woken when it exits.
func cmdWait(t *ui.Term, args []string) int {
	rest, id, timeout, err := sessionFlag(args)
	if err != nil || len(rest) > 0 {
		return fail(t, "usage: tsunagi wait [--session <id>] [--timeout 30m]")
	}
	base, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	s, err := pickSession(handoff.List(base), id, sessionPID())
	if err != nil {
		return fail(t, "%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	d, err := handoff.Wait(ctx, s.Done, time.Now(), 500*time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tsunagi: session %d did not finish within %s\n", s.PID, timeout)
		return 124
	}
	fmt.Printf("session %d finished (%s)\n%s\n", s.PID, d.Source, d.Summary)
	return 0
}

// cmdHandoff prints what a Claude session needs to pass a plan on.
func cmdHandoff(t *ui.Term, args []string) int {
	rest, id, _, err := sessionFlag(args)
	if err != nil || len(rest) > 0 {
		return fail(t, "usage: tsunagi handoff [--session <id>]")
	}
	base, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	s, err := pickSession(handoff.List(base), id, sessionPID())
	if err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("session %d  %s  %s\n\n", s.PID, s.CWD, s.TTY)
	t.Printf("1. Type the plan into that terminal tab (the terminal panel tool, or paste it yourself).\n")
	t.Printf("2. Run this in the background; it exits, and wakes you, when the session finishes its turn:\n\n")
	t.Printf("   tsunagi wait --session %d\n\n", s.PID)
	t.Printf("The session's last message comes back as the result. The finish is detected by a Stop hook,\n")
	t.Printf("so the plan needs no closing instruction. If the model should report differently, it can run:\n\n")
	t.Printf("   tsunagi done \"<summary>\"\n")
	return 0
}

// pluginArgs adds the tsunagi slash commands to the launched session through
// --plugin-dir. TSUNAGI_NO_PLUGIN turns it off.
func pluginArgs(args []string, exe string) []string {
	if os.Getenv("TSUNAGI_NO_PLUGIN") != "" {
		return args
	}
	base, err := events.Dir()
	if err != nil {
		return args
	}
	dir := filepath.Join(base, "plugin")
	if err := slash.Write(dir, exe); err != nil {
		return args
	}
	return append([]string{"--plugin-dir", dir}, args...)
}
