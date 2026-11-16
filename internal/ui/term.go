// Package ui draws tsunagi's terminal interface: plain ASCII glyphs with ANSI
// color, no dependencies.
package ui

import (
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

type RGB struct{ R, G, B uint8 }

var (
	Deep  = RGB{14, 80, 100}
	Teal  = RGB{30, 170, 150}
	Mint  = RGB{140, 255, 200}
	White = RGB{235, 245, 245}
	Water = RGB{40, 84, 136}
	Amber = RGB{255, 190, 80}
	Rose  = RGB{255, 110, 120}
	Gray  = RGB{125, 135, 145}
)

func Lerp(a, b RGB, t float64) RGB {
	t = math.Max(0, math.Min(1, t))
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return RGB{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B)}
}

// EelGrad runs tail (0) to head (1): deep water blue, teal, mint.
func EelGrad(t float64) RGB {
	if t < 0.5 {
		return Lerp(Deep, Teal, t*2)
	}
	return Lerp(Teal, Mint, (t-0.5)*2)
}

type Term struct {
	Out   io.Writer
	TTY   bool
	Color bool
	True  bool
	Anim  bool
	W, H  int
}

// Detect inspects stdout. Pipes, NO_COLOR and TERM=dumb get plain text;
// TSUNAGI_NO_ANIM=1 keeps color but drops animation.
func Detect() *Term {
	st, err := os.Stdout.Stat()
	tty := err == nil && st.Mode()&os.ModeCharDevice != 0
	t := &Term{Out: os.Stdout, TTY: tty, W: 80, H: 24}
	_, noColor := os.LookupEnv("NO_COLOR")
	t.Color = tty && !noColor && os.Getenv("TERM") != "dumb"
	ct := os.Getenv("COLORTERM")
	t.True = t.Color && (ct == "truecolor" || ct == "24bit")
	t.Anim = t.Color && os.Getenv("TSUNAGI_NO_ANIM") == ""
	if tty {
		if w, h, ok := ttySize(); ok {
			t.W, t.H = w, h
		}
	}
	return t
}

func ttySize() (w, h int, ok bool) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return 0, 0, false
	}
	defer tty.Close()
	cmd := exec.Command("stty", "size")
	cmd.Stdin = tty
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, false
	}
	if n, _ := fmt.Sscanf(string(out), "%d %d", &h, &w); n != 2 || w < 20 {
		return 0, 0, false
	}
	return w, h, true
}

func (t *Term) fg(c RGB) string {
	if !t.Color {
		return ""
	}
	if t.True {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
	}
	q := func(v uint8) int { return int(math.Round(float64(v) / 255 * 5)) }
	return fmt.Sprintf("\x1b[38;5;%dm", 16+36*q(c.R)+6*q(c.G)+q(c.B))
}

func (t *Term) Paint(c RGB, s string) string {
	if !t.Color {
		return s
	}
	return t.fg(c) + s + "\x1b[0m"
}

func (t *Term) Bold(s string) string {
	if !t.Color {
		return s
	}
	return "\x1b[1m" + s + "\x1b[0m"
}

func (t *Term) Dim(s string) string { return t.Paint(Gray, s) }

func (t *Term) Printf(format string, a ...any) { fmt.Fprintf(t.Out, format, a...) }

const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
)

// guardCursor hides the cursor and restores it on return, SIGINT or SIGTERM.
func (t *Term) guardCursor() (restore func()) { return t.guard(nil) }

// guard is guardCursor plus an extra cleanup that also runs on return and on
// those signals.
func (t *Term) guard(extra func()) (restore func()) {
	if !t.TTY {
		return func() {}
	}
	fmt.Fprint(t.Out, hideCursor)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if _, ok := <-ch; ok {
			if extra != nil {
				extra()
			}
			fmt.Fprint(t.Out, "\x1b[0m"+showCursor+"\n")
			os.Exit(130)
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
		if extra != nil {
			extra()
		}
		fmt.Fprint(t.Out, "\x1b[0m"+showCursor)
	}
}

// VisibleLen counts printed columns, ignoring ANSI escape sequences.
func VisibleLen(s string) int {
	n, esc := 0, false
	for i := 0; i < len(s); i++ {
		switch {
		case esc:
			if s[i] >= '@' && s[i] <= '~' && s[i] != '[' {
				esc = false
			}
		case s[i] == 0x1b:
			esc = true
		default:
			n++
		}
	}
	return n
}

func pad(s string, w int) string {
	if n := VisibleLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
