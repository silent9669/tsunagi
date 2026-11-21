package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Stepper draws the steps of a flow on one line with the finished ones
// checked and the current one marked: "✓ Account ─ ● Key ─ ○ Verify". When
// the line would not fit in width it shrinks to the current step and its
// position: "● Key 2/3".
func Stepper(steps []string, cur, width int, th Theme, utf8 bool) string {
	done, now, next, sep := "✓", "●", "○", " ─ "
	if !utf8 {
		done, now, next, sep = "x", "*", "o", " - "
	}
	var parts []string
	plain := 0
	for i, s := range steps {
		switch {
		case i < cur:
			parts = append(parts, th.OK.Render(done+" "+s))
		case i == cur:
			parts = append(parts, th.Accent.Render(now+" "+s))
		default:
			parts = append(parts, th.Dim.Render(next+" "+s))
		}
		plain += lipgloss.Width(s) + 2
	}
	plain += lipgloss.Width(sep) * (len(steps) - 1)
	if plain <= width {
		return strings.Join(parts, th.Dim.Render(sep))
	}
	if cur < 0 || cur >= len(steps) {
		cur = 0
	}
	return th.Accent.Render(now+" "+steps[cur]) + th.Dim.Render(fmt.Sprintf(" %d/%d", cur+1, len(steps)))
}

// Legend joins "key|action" pairs into one dim line that fits width, keeping
// the first pairs and dropping the last ones that do not fit. Key names are
// written with unicode arrows and replaced by words on an ASCII terminal.
func Legend(th Theme, utf8 bool, width int, pairs ...string) string {
	ascii := strings.NewReplacer("↑↓", "up/down", "↵", "enter", "←→", "left/right", "esc", "esc")
	var out []string
	used := 0
	for _, p := range pairs {
		k, a, _ := strings.Cut(p, "|")
		if !utf8 {
			k = ascii.Replace(k)
		}
		cell := th.Text.Render(k)
		w := lipgloss.Width(k)
		if a != "" {
			cell += " " + th.Dim.Render(a)
			w += 1 + lipgloss.Width(a)
		}
		if used+w+3*len(out) > width {
			break
		}
		out = append(out, cell)
		used += w
	}
	return strings.Join(out, "   ")
}
