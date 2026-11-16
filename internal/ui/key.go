package ui

import (
	"os"
	"os/exec"
	"strings"
)

// skipKey lets a key press end an animation. It switches the terminal to a
// mode where reads return at once, so there is no reader waiting in the
// background to swallow a later keystroke meant for Claude Code. The returned
// restore puts the terminal back and discards what was typed during the
// animation. Without a terminal on stdin both are no-ops.
func (t *Term) skipKey() (pressed func() bool, restore func()) {
	noop := func() bool { return false }
	st, err := os.Stdin.Stat()
	if !t.TTY || err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return noop, func() {}
	}
	get := exec.Command("stty", "-g")
	get.Stdin = os.Stdin
	saved, err := get.Output()
	if err != nil {
		return noop, func() {}
	}
	stty := func(args ...string) error {
		c := exec.Command("stty", args...)
		c.Stdin = os.Stdin
		return c.Run()
	}
	if stty("-icanon", "-echo", "min", "0", "time", "0") != nil {
		return noop, func() {}
	}
	buf := make([]byte, 64)
	read := func() int { n, _ := os.Stdin.Read(buf); return n }
	pressed = func() bool { return read() > 0 }
	restore = func() {
		for read() > 0 { // drop keys typed during the animation
		}
		stty(strings.TrimSpace(string(saved)))
	}
	return pressed, restore
}
