package app

import (
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/ui"
)

// titled is implemented by screens that replace the wordmark and the sea with
// a one-row title, leaving the terminal to their list.
type titled interface{ title() string }

// legendary is implemented by screens that name their keys on the last row.
type legendary interface{ legend() []string }

// blockWidth is the width of the one column every screen's body lives in.
func blockWidth(w int) int { return max(min(w-4, 56), min(w, 20)) }

// frame is how the terminal is divided: header rows (the sea, or a title),
// the body the screen draws in, and a key legend on the last row. It depends
// only on the terminal size and the kind of screen, so a screen can be sized
// once and mouse clicks mapped back to its rows.
type frame struct {
	W, H       int
	tier       int // 0 wordmark and sea, 1 small sea, 2 one brand line, -1 title row
	headerRows int
	seaRows    int
	footerRows int
	bodyW      int
	bodyH      int
	indent     int
}

func (a *App) frame(scr tea.Model) frame {
	W, H := a.opts.W, a.opts.H
	if W <= 0 || H <= 0 {
		W, H = 80, 24
	}
	f := frame{W: W, H: H, bodyW: blockWidth(W)}
	f.indent = max((W-f.bodyW)/2, 0)
	if H >= 10 {
		f.footerRows = 1
	}
	_, isTitled := scr.(titled)
	switch {
	case isTitled:
		f.tier = -1
		f.headerRows = 1
		if H >= 10 {
			f.headerRows = 2
		}
	case H >= 30 && W >= 60:
		f.tier, f.seaRows = 0, 8
		f.headerRows = len(ui.LogoLines()) + 1 + f.seaRows + 1
	case H >= 20 && W >= 40:
		f.tier, f.seaRows = 1, 4
		f.headerRows = 1 + f.seaRows + 1
	default:
		f.tier = 2
		f.headerRows = 1
		if H >= 10 {
			f.headerRows = 2
		}
	}
	f.bodyH = max(H-f.headerRows-f.footerRows, 1)
	return f
}

// bodyMouse maps a click or wheel event from terminal cells to the body's
// own cells. Clicks outside the body are dropped; the wheel works anywhere.
func (a *App) bodyMouse(msg tea.Msg) (tea.Msg, bool) {
	f := a.frame(a.scr)
	switch v := msg.(type) {
	case tea.MouseClickMsg:
		mo := tea.Mouse(v)
		mo.X -= f.indent
		mo.Y -= f.headerRows
		if mo.X < 0 || mo.X >= f.bodyW || mo.Y < 0 || mo.Y >= f.bodyH {
			return nil, false
		}
		return tea.MouseClickMsg(mo), true
	case tea.MouseWheelMsg:
		mo := tea.Mouse(v)
		mo.X, mo.Y = 0, 0
		return tea.MouseWheelMsg(mo), true
	}
	return nil, false
}

// sea is the animated sea scene sized for a frame, centered in the width.
func (a *App) sea(f frame) []string {
	ao := a.opts
	ao.W, ao.H = min(f.W-2, 72), f.seaRows
	phase := a.phase
	if !a.opts.Anim {
		phase = artStillPhase
	}
	pad := strings.Repeat(" ", max((f.W-ao.W)/2, 0))
	var out []string
	for _, l := range strings.Split(tui.OceanArt(ao, a.theme, phase), "\n") {
		out = append(out, pad+l)
	}
	return out
}

const artStillPhase = 36.0 // art.OceanStaticPhase

// View composes the header, the screen's body and the key legend, always
// inside the terminal: no row is wider than W and there are never more than
// H rows.
func (a *App) View() tea.View {
	f := a.frame(a.scr)
	var body string
	if a.scr != nil {
		body = strings.TrimRight(a.scr.View().Content, "\n")
	}
	indent := strings.Repeat(" ", f.indent)

	var lines []string
	switch f.tier {
	case -1:
		lines = append(lines, indent+a.theme.Title.Render(a.scr.(titled).title()))
	case 0:
		lines = append(lines, tui.Wordmark(f.W, a.theme)...)
		lines = append(lines, "")
		lines = append(lines, a.sea(f)...)
	case 1:
		lines = append(lines, indent+a.theme.Title.Render("tsunagi"))
		lines = append(lines, a.sea(f)...)
	default:
		lines = append(lines, indent+a.theme.Title.Render("tsunagi"))
	}
	for len(lines) < f.headerRows {
		lines = append(lines, "")
	}
	lines = lines[:f.headerRows]

	bl := strings.Split(body, "\n")
	if len(bl) > f.bodyH {
		bl = bl[:f.bodyH]
	}
	for _, l := range bl {
		lines = append(lines, indent+l)
	}
	if f.footerRows > 0 {
		for len(lines) < f.H-1 {
			lines = append(lines, "")
		}
		lines = lines[:f.H-1]
		lines = append(lines, indent+a.legendLine(f))
	}
	clip := lipgloss.NewStyle().MaxWidth(f.W)
	for i, l := range lines {
		if lipgloss.Width(l) > f.W {
			lines[i] = clip.Render(l)
		}
	}
	if len(lines) > f.H {
		lines = lines[:f.H]
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

// legendLine names the keys of the current screen, as many as fit.
func (a *App) legendLine(f frame) string {
	pairs := []string{"↑↓|move", "↵|select", "esc|back"}
	if l, ok := a.scr.(legendary); ok {
		pairs = l.legend()
	}
	return tui.Legend(a.theme, a.opts.UTF8, f.bodyW, pairs...)
}
