package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/art"
)

// wordmark is "TSUNAGI" in the ANSI-Shadow block font, six rows.
var wordmark = []string{
	"████████╗███████╗██╗   ██╗███╗   ██╗ █████╗  ██████╗ ██╗",
	"╚══██╔══╝██╔════╝██║   ██║████╗  ██║██╔══██╗██╔════╝ ██║",
	"   ██║   ███████╗██║   ██║██╔██╗ ██║███████║██║  ███╗██║",
	"   ██║   ╚════██║██║   ██║██║╚██╗██║██╔══██║██║   ██║██║",
	"   ██║   ███████║╚██████╔╝██║ ╚████║██║  ██║╚██████╔╝██║",
	"   ╚═╝   ╚══════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝ ╚═════╝ ╚═╝",
}

// Wordmark renders the block-font wordmark with a horizontal gradient and
// centered in w cells. Non-UTF-8 terminals get a one-row bold line; mono
// themes get the glyphs unstyled.
func Wordmark(w int, th Theme, utf8 bool) []string {
	if !utf8 {
		return []string{centerLineRaw(th.Title.Render("tsunagi"), w, 7)}
	}
	var out []string
	for _, line := range wordmark {
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
