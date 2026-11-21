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
	"github.com/silent9669/tsunagi/internal/ui"
)

const (
	altOn  = "\x1b[?1049h\x1b[?25l"
	altOff = "\x1b[?25h\x1b[?1049l"
	watchH = 3 // rows of swimming eel at the top
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
	fmt.Fprint(t.Out, altOn)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer fmt.Fprint(t.Out, altOff)

	var (
		tails  = map[string]*events.Tail{} // one per session's event file
		log    []events.Event
		counts = map[string]int{}
		start  = time.Now()
	)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	eelX := 0.0
	for n := 0; ; n++ {
		select {
		case <-ch:
			return 0
		case <-tick.C:
		}
		if n%20 == 0 { // look for new sessions once a second
			for _, p := range events.Recent(dir, time.Hour) {
				if tails[p] == nil {
					tails[p] = &events.Tail{Path: p}
				}
			}
		}
		fresh := false
		for _, tl := range tails {
			for _, e := range tl.Poll() {
				if time.Since(e.Time) > 10*time.Minute {
					continue // history from before watch started
				}
				fresh = true
				log = append(log, e)
				counts[acctOf(e)]++
			}
		}
		if fresh {
			sort.Slice(log, func(i, j int) bool { return log[i].Time.Before(log[j].Time) })
		}
		if len(log) > 500 {
			log = log[len(log)-500:]
		}
		// the eel swims faster while requests are flowing
		eelX += 1.5 + 0.9*float64(recent(log, 3*time.Second))
		t.Out.Write([]byte(renderWatch(t, time.Since(start).Seconds(), int(eelX), len(tails) > 0, log, counts)))
	}
}

func renderWatch(t *ui.Term, tsec float64, eelX int, live bool, log []events.Event, counts map[string]int) string {
	w, h := t.W, t.H
	c := ui.NewCanvas(w, watchH)
	ui.DrawWater(c, tsec)
	span := w + ui.EelLen + 6
	ui.DrawEel(c, eelX%span-2, 1, 0.9, tsec*8, ui.Blinks(tsec))
	lines := c.Lines(t)

	state := "waiting for a session"
	if live {
		state = fmt.Sprintf("%d requests", len(log))
	}
	lines = append(lines, "  "+t.Bold("tsunagi watch")+"  "+t.Dim(state))

	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Strings(names)
	var chips []string
	for _, n := range names {
		chips = append(chips, t.Paint(ui.Teal, clean(n))+" "+fmt.Sprint(counts[n]))
	}
	lines = append(lines, "  "+strings.Join(chips, "   "))
	lines = append(lines, "  "+t.Dim("traffic ")+t.Paint(ui.Teal, sparkline(log, time.Now(), min(w-14, 60), 5*time.Second))+t.Dim("  5 min"), "")

	lines = append(lines,
		"  "+t.Dim(fmt.Sprintf("%-9s %-16s %-26s %-4s %-6s %s", "time", "account", "model", "http", "ms", "tokens in/out")),
		"  "+t.Dim(strings.Repeat("-", min(w-4, 78))))
	rows := h - len(lines) - 1
	if rows < 0 {
		rows = 0
	}
	if len(log) > rows {
		log = log[len(log)-rows:]
	}
	for _, e := range log {
		model := clean(e.Model)
		if model == "" {
			model = "-"
		}
		acct := e.Account
		if acct == "" {
			acct = e.Provider
		}
		failover := ""
		if e.Skipped > 0 {
			failover = t.Paint(ui.Amber, fmt.Sprintf("  +%d failover", e.Skipped))
		}
		tokens := "-"
		if e.In+e.Out > 0 {
			tokens = ui.Count(e.In) + "/" + ui.Count(e.Out)
		}
		lines = append(lines, fmt.Sprintf("  %-9s %-16s %-26s %s %-6d %s%s",
			e.Time.Format("15:04:05"), clip(clean(acct), 16), clip(model, 26),
			pad4(t, e.Status), e.Millis, tokens, failover))
	}

	var b strings.Builder
	b.WriteString("\x1b[H")
	for _, l := range lines {
		b.WriteString(clipVisible(l, w) + "\x1b[K\n")
	}
	b.WriteString("\x1b[J")
	return b.String()
}

func pad4(t *ui.Term, status int) string {
	s := fmt.Sprintf("%-4d", status)
	switch {
	case status >= 500:
		return t.Paint(ui.Rose, s)
	case status >= 400:
		return t.Paint(ui.Amber, s)
	}
	return t.Paint(ui.Mint, s)
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

// clipVisible cuts a line to n printed columns, keeping escape sequences whole.
func clipVisible(s string, n int) string {
	cols, esc := 0, false
	for i := 0; i < len(s); i++ {
		switch {
		case esc:
			if s[i] >= '@' && s[i] <= '~' && s[i] != '[' {
				esc = false
			}
		case s[i] == 0x1b:
			esc = true
		default:
			if cols == n {
				return s[:i] + "\x1b[0m"
			}
			cols++
		}
	}
	return s
}

func acctOf(e events.Event) string {
	if e.Account != "" {
		return e.Account
	}
	return e.Provider
}

// recent counts events newer than d.
func recent(log []events.Event, d time.Duration) int {
	n := 0
	for _, e := range log {
		if time.Since(e.Time) < d {
			n++
		}
	}
	return n
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
