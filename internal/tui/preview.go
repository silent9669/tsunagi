package tui

import (
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/ui"
)

// Preview is the appearance preview behind `tsunagi look`: the wordmark over
// the animated eel, with left/right cycling the art look and up/down cycling
// the theme. It is also the Appearance screen step 4 builds on.
type Preview struct {
	opts     Options
	theme    Theme
	lookIdx  int
	themeIdx int
	phase    float64
}

type previewTick time.Time

// NewPreview builds the preview for resolved options.
func NewPreview(o Options) (Preview, error) {
	th, err := o.ThemeOr()
	if err != nil {
		return Preview{}, err
	}
	p := Preview{opts: o, theme: th}
	for i, l := range art.Looks {
		if l == o.Look {
			p.lookIdx = i
		}
	}
	for i, n := range ThemeNames {
		if n == o.Theme {
			p.themeIdx = i
		}
	}
	return p, nil
}

// Look and Theme report the current selection (for tests and callers).
func (p Preview) Look() art.Look { return art.Looks[p.lookIdx] }
func (p Preview) Theme() string  { return ThemeNames[p.themeIdx] }

func (p Preview) Init() tea.Cmd {
	if !p.opts.Anim {
		return nil
	}
	return tea.Tick(artTick, func(t time.Time) tea.Msg { return previewTick(t) })
}

func (p Preview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		p.opts.W, p.opts.H = v.Width, v.Height
		return p, nil
	case previewTick:
		p.phase += 0.4
		if !p.opts.Anim {
			return p, nil
		}
		return p, tea.Tick(artTick, func(t time.Time) tea.Msg { return previewTick(t) })
	case tea.KeyPressMsg:
		code, text := v.Code, v.Text
		switch {
		case code == tea.KeyLeft || text == "h":
			p.cycleLook(-1)
		case code == tea.KeyRight || text == "l":
			p.cycleLook(1)
		case code == tea.KeyUp || text == "k":
			p.cycleTheme(-1)
		case code == tea.KeyDown || text == "j":
			p.cycleTheme(1)
		case text == "q" || code == tea.KeyEscape || (code == 'c' && v.Mod&tea.ModCtrl != 0):
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p *Preview) cycleLook(d int) {
	n := len(art.Looks)
	p.lookIdx = (p.lookIdx + d + n) % n
	p.opts.Look = art.Looks[p.lookIdx]
}

func (p *Preview) cycleTheme(d int) {
	n := len(ThemeNames)
	p.themeIdx = (p.themeIdx + d + n) % n
	p.opts.Theme = ThemeNames[p.themeIdx]
	if th, err := NewTheme(p.opts.Theme, p.opts.Profile()); err == nil {
		p.theme = th
	}
}

// PreviewFrame renders one static frame: wordmark, eel and the current
// look/theme names on a dim line. Used by --static and by View. The eel gets
// whatever rows the wordmark and status line leave.
func PreviewFrame(o Options, th Theme, phase float64) string {
	var sb strings.Builder
	reserved := 1 // the status line
	if !Compact(o.W, o.H) {
		reserved += 7 // six logo rows plus a blank separator
	}
	if !Compact(o.W, o.H) {
		for _, line := range ui.LogoLines() {
			sb.WriteString(centerLine(line, o.W, th))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	artOpts := o
	artOpts.H = o.H - reserved
	if artOpts.H < 4 {
		artOpts.H = 4
	}
	sb.WriteString(renderArt(artOpts, th, phase))
	sb.WriteByte('\n')
	sb.WriteString(th.Dim.Render(string(o.Look) + " " + o.Theme))
	return sb.String()
}

// centerLine pads a wordmark row so it sits in the middle of the width.
func centerLine(line string, w int, th Theme) string {
	lw := lipgloss.Width(line)
	if lw >= w {
		return logoLine(line, th)
	}
	pad := (w - lw) / 2
	return strings.Repeat(" ", pad) + logoLine(line, th)
}

// logoLine colors each wordmark character along the theme's art gradient.
func logoLine(line string, th Theme) string {
	maxW := 0
	for _, l := range ui.LogoLines() {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}
	if maxW < 1 {
		maxW = 1
	}
	var sb strings.Builder
	x := 0
	for _, r := range line {
		if c := th.Gradient(float64(x) / float64(maxW)); c != nil {
			sb.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(r)))
		} else {
			sb.WriteRune(r)
		}
		x++
	}
	return sb.String()
}

func (p Preview) View() tea.View {
	content := PreviewFrame(p.opts, p.theme, p.phase)
	return tea.NewView(Place(p.opts.W, p.opts.H, content))
}
