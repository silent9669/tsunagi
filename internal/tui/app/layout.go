package app

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/tui"
)

// titled is implemented by screens whose breadcrumb is just their name.
type titled interface{ title() string }

// crumber is implemented by screens that carry a deeper breadcrumb path.
type crumber interface{ crumbs() []string }

// legendary is implemented by screens that name their keys on the last row.
type legendary interface{ legend() []string }

// wizStager is implemented by wizard screens that show the progress rail.
type wizStager interface{ wizStage() (cur int, on bool) }

// blockWidth is the width of the one column every screen's body lives in.
func blockWidth(w int) int { return max(min(w-4, 108), min(w, 20)) }

// frame is how the terminal is divided: the brand bar and optional banner or
// wizard rail, the body the screen draws in, and a key legend on the last
// row. It depends only on the terminal size and the kind of screen.
type frame struct {
	W, H       int
	headerRows int
	seaRows    int
	banner     int // 0 none, 1 wordmark+sea, 2 sea, 3 small sea
	wizRail    int // wizard progress step, -1 none
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
	f := frame{W: W, H: H, bodyW: blockWidth(W), wizRail: -1}
	f.indent = max((W-f.bodyW)/2, 0)
	f.headerRows = 1 // brand bar
	if w, ok := scr.(wizStager); ok {
		if cur, on := w.wizStage(); on {
			f.wizRail = cur
			f.headerRows++
		}
	}
	_, isHome := scr.(*homeScreen)
	if isHome {
		wmRows := 6 // ANSI-Shadow wordmark
		if !a.opts.UTF8 {
			wmRows = 1 // a single bold "tsunagi" line
		}
		switch {
		case H >= 40 && W >= 62:
			f.banner, f.seaRows = 1, 5
			f.headerRows += 1 + wmRows + 1 + f.seaRows
		case H >= 26 && W >= 40:
			f.banner, f.seaRows = 2, 5
			f.headerRows += 1 + f.seaRows
		case H >= 20 && W >= 40:
			f.banner, f.seaRows = 3, 3
			f.headerRows += 1 + f.seaRows
		}
	}
	f.headerRows++ // blank separator
	if H >= 8 {
		f.footerRows = 1
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

// crumbOf is the right side of the brand bar on non-home screens.
func (a *App) crumbOf(scr tea.Model) string {
	var parts []string
	if c, ok := scr.(crumber); ok {
		parts = c.crumbs()
	} else if t, ok := scr.(titled); ok {
		parts = []string{t.title()}
	}
	sep := " › "
	if !a.opts.UTF8 {
		sep = " > "
	}
	return strings.Join(parts, sep)
}

// homeChips is the right side of the brand bar on the home screen.
func (a *App) homeChips() string {
	check, cross, dot := "✓", "✗", "·"
	if !a.opts.UTF8 {
		check, cross, dot = "+", "x", "-"
	}
	claude := "claude " + check
	if a.snap.ClaudePath == "" {
		claude = "claude " + cross
	}
	chips := claude
	ready := 0
	for _, p := range a.snap.Providers {
		if p.Ready && p.Group != "local" {
			ready++
		}
	}
	if ready > 0 {
		chips += " " + dot + " " + itoa(ready) + " ready"
	}
	if n := len(a.snap.FreeTargets); n > 0 {
		chips += " " + dot + " free/auto " + itoa(n)
	}
	return chips
}

// wizRailLine renders the wizard progress rail and collapses to "◆ n/3"
// when narrow.
func (a *App) wizRailLine(cur int, w int) string {
	steps := []string{"Providers", "Skills", "Ready"}
	done, now, next, sep := "◇", "◆", "○", " ─ "
	if !a.opts.UTF8 {
		done, now, next, sep = "o", "*", ".", " - "
	}
	var parts []string
	plain := 0
	for i, s := range steps {
		glyph, st := next, a.theme.Dim
		switch {
		case i < cur:
			glyph, st = done, a.theme.Dim
		case i == cur:
			glyph, st = now, a.theme.Accent
		}
		parts = append(parts, st.Render(glyph+" "+s))
		plain += lipgloss.Width(s) + 2
	}
	if plain+lipgloss.Width(sep)*(len(steps)-1) <= w {
		return strings.Join(parts, a.theme.Dim.Render(sep))
	}
	if cur < 0 || cur >= len(steps) {
		cur = 0
	}
	return a.theme.Accent.Render(now+" "+steps[cur]) + a.theme.Dim.Render(fmt.Sprintf(" %d/%d", cur+1, len(steps)))
}

// View composes the brand bar, the screen's body and the key legend, always
// inside the terminal: no row is wider than W and there are never more than
// H rows.
func (a *App) View() tea.View {
	f := a.frame(a.scr)
	var body string
	if a.scr != nil {
		body = strings.TrimRight(a.scr.View().Content, "\n")
	}
	indent := strings.Repeat(" ", f.indent)

	right := a.crumbOf(a.scr)
	if _, isHome := a.scr.(*homeScreen); isHome {
		right = a.homeChips()
	}
	lines := []string{a.theme.BrandBar(Version, right, f.W, a.opts.UTF8)}
	if f.wizRail >= 0 {
		lines = append(lines, indent+a.wizRailLine(f.wizRail, f.bodyW))
	}
	switch f.banner {
	case 1:
		lines = append(lines, "")
		lines = append(lines, tui.Wordmark(f.W, a.theme, a.opts.UTF8)...)
		lines = append(lines, "")
		lines = append(lines, a.sea(f)...)
	case 2, 3:
		lines = append(lines, "")
		lines = append(lines, a.sea(f)...)
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
