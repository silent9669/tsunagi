package ui

import (
	"fmt"
	"strings"
	"time"
)

const routesDur = 0.8 // seconds

// RoutesFrame draws where requests can go: Claude Code on the left, tsunagi in
// the middle, Claude and each ready provider on the right, with pulses running
// along the links. final is the still picture left in the scrollback.
func (t *Term) RoutesFrame(tsec float64, targets []string, final bool) []string {
	rows := len(targets)
	if rows == 0 {
		rows = 1
	}
	mid := " (tsunagi) "
	lines := make([]string, rows)
	pulse := func(n, off int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			if !final && (i+off+int(tsec*22))%6 == 0 {
				b.WriteString(t.Paint(Mint, ">"))
			} else {
				b.WriteString(t.Paint(Teal, "="))
			}
		}
		return b.String()
	}
	for r := 0; r < rows; r++ {
		var l string
		switch {
		case r == rows/2:
			l = t.Paint(White, "claude ") + pulse(2, 0) + t.Paint(Mint, mid) + pulse(2, r)
		default:
			l = strings.Repeat(" ", len("claude ")+2+len(mid)+2)
		}
		join := "+"
		if rows == 1 {
			join = "="
		}
		name := ""
		if r < len(targets) {
			name = targets[r]
		}
		lines[r] = "  " + l + t.Paint(Teal, join) + pulse(3, r+1) + t.Paint(Mint, ">") + " " + name
	}
	return lines
}

// Routes plays the link animation, or prints the still picture. A key press
// skips it.
func (t *Term) Routes(targets []string) {
	if !t.TTY {
		return
	}
	n := len(targets)
	if n == 0 {
		n = 1
	}
	if !t.Anim {
		t.draw(t.RoutesFrame(0, targets, true))
		return
	}
	pressed, unraw := t.skipKey()
	restore := t.guard(unraw)
	defer restore()
	start := time.Now()
	first := true
	for {
		el := time.Since(start).Seconds()
		if el >= routesDur || pressed() {
			break
		}
		if !first {
			fmt.Fprintf(t.Out, "\x1b[%dA", n)
		}
		t.draw(t.RoutesFrame(el, targets, false))
		first = false
		time.Sleep(frameDelay)
	}
	if !first {
		fmt.Fprintf(t.Out, "\x1b[%dA", n)
	}
	t.draw(t.RoutesFrame(0, targets, true))
}
