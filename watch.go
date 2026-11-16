package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

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
func cmdWatch(t *ui.Term) int {
	if !t.TTY {
		return fail(t, "watch needs a terminal")
	}
	dir, err := events.Dir()
	if err != nil {
		return fail(t, "%v", err)
	}
	fmt.Fprint(t.Out, altOn)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer fmt.Fprint(t.Out, altOff)

	var (
		tail   events.Tail
		log    []events.Event
		counts = map[string]int{}
		start  = time.Now()
	)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ch:
			return 0
		case <-tick.C:
		}
		if p := events.Latest(dir); p != tail.Path {
			tail = events.Tail{Path: p}
			log, counts = nil, map[string]int{}
		}
		for _, e := range tail.Poll() {
			log = append(log, e)
			counts[e.Provider]++
		}
		if len(log) > 500 {
			log = log[len(log)-500:]
		}
		t.Out.Write([]byte(renderWatch(t, time.Since(start).Seconds(), tail.Path != "", log, counts)))
	}
}

func renderWatch(t *ui.Term, tsec float64, live bool, log []events.Event, counts map[string]int) string {
	w, h := t.W, t.H
	c := ui.NewCanvas(w, watchH)
	ui.DrawWater(c, tsec)
	span := w + ui.EelLen + 6
	ui.DrawEel(c, int(tsec*30)%span-2, 1, 0.9, tsec*8, ui.Blinks(tsec))
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
		chips = append(chips, t.Paint(ui.Teal, n)+" "+fmt.Sprint(counts[n]))
	}
	lines = append(lines, "  "+strings.Join(chips, "   "), "")

	lines = append(lines,
		"  "+t.Dim(fmt.Sprintf("%-9s %-12s %-28s %-4s %s", "time", "provider", "model", "http", "ms")),
		"  "+t.Dim(strings.Repeat("-", min(w-4, 62))))
	rows := h - len(lines) - 1
	if rows < 0 {
		rows = 0
	}
	if len(log) > rows {
		log = log[len(log)-rows:]
	}
	for _, e := range log {
		model := e.Model
		if model == "" {
			model = "-"
		}
		lines = append(lines, fmt.Sprintf("  %-9s %-12s %-28s %s %d",
			e.Time.Format("15:04:05"), clip(e.Provider, 12), clip(model, 28),
			pad4(t, e.Status), e.Millis))
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
