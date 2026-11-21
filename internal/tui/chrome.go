package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/ui"
)

// Wordmark renders the six logo rows gradient-colored and centered in w
// cells. It returns nil when the terminal is compact.
func Wordmark(w int, th Theme) []string {
	if Compact(w, 24) {
		return nil
	}
	var out []string
	for _, line := range ui.LogoLines() {
		out = append(out, centerLine(line, w, th))
	}
	return out
}

// EelArt renders the animated eel scene for the given options and phase. The
// caller picks the Options (W/H bound the art area).
func EelArt(o Options, th Theme, phase float64) string {
	return renderArt(o, th, phase)
}

// OceanArt renders the sea scene with the swimming eel. phase moves it; a
// still frame (animation off) passes art.OceanStaticPhase.
func OceanArt(o Options, th Theme, phase float64) string {
	return renderScene(o, th, phase, art.Ocean)
}

// JoinLines trims trailing blank lines off a block of text.
func JoinLines(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimRight(p, "\n")
		if p == "" {
			continue
		}
		b.WriteString(p)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// WidthOf reports the terminal width lipgloss sees for s.
func WidthOf(s string) int { return lipgloss.Width(s) }
