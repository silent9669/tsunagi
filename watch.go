package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/tui/dash"
	"github.com/silent9669/tsunagi/internal/ui"
)

// cmdWatch shows live traffic from the newest running session. It reads the
// events file the router writes; it never talks to the router.
func cmdWatch(t *ui.Term, args []string) int {
	args, logMode := popFlag(args, "--log")
	args, jsonMode := popFlag(args, "--json")
	if len(args) > 0 || (logMode && jsonMode) {
		return fail(t, "usage: tsunagi watch [--log | --json]")
	}
	dir, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	if logMode || jsonMode {
		return watchStream(t, dir, jsonMode)
	}
	if !t.TTY {
		return fail(t, "the dashboard needs a terminal. Use: tsunagi watch --log")
	}
	opts, err := appOptions(t)
	if err != nil {
		return fail(t, "%v", err)
	}
	m, err := dash.New(opts, &watchSource{dir: dir, tails: map[string]*events.Tail{}})
	if err != nil {
		return fail(t, "%v", err)
	}
	if _, err := tui.Run(m, opts); err != nil {
		return fail(t, "%v", err)
	}
	return 0
}

// watchSource feeds the dashboard from every recent session's events file.
type watchSource struct {
	dir   string
	tails map[string]*events.Tail
	n     int
}

func (w *watchSource) Poll() ([]events.Event, bool) {
	if w.n%12 == 0 { // look for new sessions about once a second
		for _, p := range events.Recent(w.dir, time.Hour) {
			if w.tails[p] == nil {
				w.tails[p] = &events.Tail{Path: p}
			}
		}
	}
	w.n++
	var out []events.Event
	for _, tl := range w.tails {
		for _, e := range tl.Poll() {
			if time.Since(e.Time) > 10*time.Minute {
				continue // history from before watch started
			}
			out = append(out, e)
		}
	}
	return out, len(w.tails) > 0
}

// clean drops control characters: model names come from request bodies and
// must not be able to inject terminal escape sequences.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
}

func acctOf(e events.Event) string {
	if e.Account != "" {
		return e.Account
	}
	return e.Provider
}

// sparkline draws requests per bucket, oldest to newest, with a ramp of ASCII
// characters from quiet to busy.
func sparkline(log []events.Event, now time.Time, buckets int, step time.Duration) string {
	if buckets < 1 {
		return ""
	}
	counts := make([]int, buckets)
	peak := 0
	for _, e := range log {
		age := int(now.Sub(e.Time) / step)
		if age >= 0 && age < buckets {
			counts[buckets-1-age]++
			if counts[buckets-1-age] > peak {
				peak = counts[buckets-1-age]
			}
		}
	}
	const ramp = " .:-=+*#"
	b := make([]byte, buckets)
	for i, n := range counts {
		switch {
		case n == 0:
			b[i] = '_'
		default:
			b[i] = ramp[max(1, n*(len(ramp)-1)/peak)]
		}
	}
	return string(b)
}

// watchStream prints each request as one line (or one JSON object) as it
// happens. It needs no terminal, so it can be piped, logged or grepped.
func watchStream(t *ui.Term, dir string, asJSON bool) int {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	tails := map[string]*events.Tail{}
	for n := 0; ; n++ {
		if n%4 == 0 {
			for _, p := range events.Recent(dir, time.Hour) {
				if tails[p] == nil {
					tl := &events.Tail{Path: p}
					tl.Poll() // skip what happened before watch started
					tails[p] = tl
				}
			}
		}
		var fresh []events.Event
		for _, tl := range tails {
			fresh = append(fresh, tl.Poll()...)
		}
		sort.Slice(fresh, func(i, j int) bool { return fresh[i].Time.Before(fresh[j].Time) })
		for _, e := range fresh {
			if asJSON {
				b, _ := json.Marshal(e)
				fmt.Fprintf(t.Out, "%s\n", b)
			} else {
				fmt.Fprintln(t.Out, eventLine(e))
			}
		}
		select {
		case <-ch:
			return 0
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// eventLine is one request as plain text, with control characters removed.
func eventLine(e events.Event) string {
	model := clean(e.Model)
	if model == "" {
		model = "-"
	}
	tokens := "-"
	if e.In+e.Out > 0 {
		tokens = fmt.Sprintf("%d/%d", e.In, e.Out)
	}
	line := fmt.Sprintf("%s %s %s %d %dms tokens=%s", e.Time.Format("15:04:05"), clean(acctOf(e)), model, e.Status, e.Millis, tokens)
	if e.Skipped > 0 {
		line += fmt.Sprintf(" failover=%d", e.Skipped)
	}
	if e.Via != "" {
		line += " via=" + clean(e.Via)
	}
	if e.Why != "" {
		line += " why=" + clean(e.Why)
	}
	return line
}
