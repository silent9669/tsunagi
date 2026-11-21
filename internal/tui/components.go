package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/silent9669/tsunagi/internal/art"
)

// Skipped is emitted by ArtModel on any keypress so a splash screen can
// dismiss itself.
type Skipped struct{}

// tickMsg drives animation frames.
type tickMsg time.Time

const artTick = 83 * time.Millisecond // ~12 fps

// ArtModel renders the procedural eel scene, animated when Options.Anim is
// set and frozen at phase 0.25 otherwise.
type ArtModel struct {
	opts  Options
	theme Theme
	phase float64
}

// NewArtModel builds the art view for the given options.
func NewArtModel(o Options) (ArtModel, error) {
	th, err := o.ThemeOr()
	if err != nil {
		return ArtModel{}, err
	}
	return ArtModel{opts: o, theme: th}, nil
}

// Phase reports the current animation phase (for tests and callers).
func (m ArtModel) Phase() float64 { return m.phase }

func (m ArtModel) Init() tea.Cmd {
	if !m.opts.Anim {
		return nil
	}
	return tea.Tick(artTick, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m ArtModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tickMsg:
		m.phase += 0.4
		if !m.opts.Anim {
			return m, nil
		}
		return m, tea.Tick(artTick, func(t time.Time) tea.Msg { return tickMsg(t) })
	case tea.KeyPressMsg:
		return m, func() tea.Msg { return Skipped{} }
	case tea.WindowSizeMsg:
		return m, nil
	}
	return m, nil
}

// View renders the eel as terminal cells. Subject cells are colored along
// the theme's art gradient by their position T; decorations stay dim. When
// the terminal is too small for art it renders nothing.
func (m ArtModel) View() tea.View {
	if Compact(m.opts.W, m.opts.H) {
		return tea.NewView("")
	}
	phase := m.phase
	if !m.opts.Anim {
		phase = 0.25
	}
	return tea.NewView(renderArt(m.opts, m.theme, phase))
}

// renderArt draws the eel scene for the options' look/size/UTF-8 settings.
func renderArt(o Options, th Theme, phase float64) string {
	look := o.Look
	if !o.UTF8 && (look == art.Braille || look == art.Bricks) {
		look = art.Dither
	}
	dx, dy := density(look)
	w, h := o.W, o.H
	bm := art.Eel(w*dx, h*dy, phase)
	cells := art.Render(bm, look, o.UTF8)

	// gradient styles quantized to 33 buckets along T
	var grad [33]lipgloss.Style
	var gradOK bool
	getGrad := func(t float64) lipgloss.Style {
		if !gradOK {
			for i := range grad {
				if c := th.Gradient(float64(i) / 32); c != nil {
					grad[i] = lipgloss.NewStyle().Foreground(c)
				}
			}
			gradOK = true
		}
		i := int(t*32 + 0.5)
		if i < 0 {
			i = 0
		}
		if i > 32 {
			i = 32
		}
		return grad[i]
	}

	var sb strings.Builder
	for _, row := range cells {
		for _, c := range row {
			if !c.On {
				sb.WriteByte(' ')
				continue
			}
			if c.T >= 0 {
				if col := getGrad(c.T); col.GetForeground() != nil {
					sb.WriteString(col.Render(string(c.R)))
					continue
				}
			}
			sb.WriteRune(c.R)
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// density mirrors art's per-look pixel block size.
func density(l art.Look) (dx, dy int) {
	switch l {
	case art.Bricks, art.Gradient:
		return 1, 2
	}
	return 2, 4
}

// Spinner is a small animated indicator.
type Spinner struct {
	theme Theme
	utf8  bool
	frame int
}

var (
	spinASCII = []rune{'|', '/', '-', '\\'}
	spinUTF8  = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
)

// NewSpinner builds a spinner in the theme's accent color.
func NewSpinner(th Theme, utf8 bool) Spinner {
	return Spinner{theme: th, utf8: utf8}
}

func (s Spinner) Init() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (s Spinner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tickMsg); ok {
		s.frame++
		return s, tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
	}
	return s, nil
}

func (s Spinner) View() tea.View {
	frames := spinASCII
	if s.utf8 {
		frames = spinUTF8
	}
	return tea.NewView(s.theme.Accent.Render(string(frames[s.frame%len(frames)])))
}

// Gauge renders the [####------] share bar used across the interface.
type Gauge struct {
	Frac  float64
	Width int
	theme Theme
}

// NewGauge builds a gauge; width is the number of fill cells.
func NewGauge(frac float64, width int, th Theme) Gauge {
	return Gauge{Frac: frac, Width: width, theme: th}
}

func (g Gauge) Init() tea.Cmd { return nil }

func (g Gauge) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return g, nil }

// View renders "[████------] NN%" with the fill colored by how much remains:
// accent above half, warn above a fifth, err below.
func (g Gauge) View() tea.View {
	frac := g.Frac
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	w := g.Width
	if w < 4 {
		w = 4
	}
	n := int(frac*float64(w) + 0.5)
	st := g.theme.Accent
	switch {
	case frac < 0.2:
		st = g.theme.Err
	case frac < 0.5:
		st = g.theme.Warn
	}
	fill := strings.Repeat("#", n)
	empty := strings.Repeat("-", w-n)
	pct := fmt.Sprintf(" %3d%%", int(frac*100+0.5))
	return tea.NewView(g.theme.Border.Render("[") + st.Render(fill) + g.theme.Border.Render(empty+"]") + g.theme.Dim.Render(pct))
}

// Frame centers content inside the terminal. Compact reports whether the
// terminal is too small for the full layout (art hidden).
func Compact(w, h int) bool { return w < 60 || h < 20 }

// Place centers content horizontally and vertically inside w by h cells.
func Place(w, h int, content string) string {
	if w <= 0 || h <= 0 {
		return content
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}
