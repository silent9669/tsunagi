package app

import (
	"strings"
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/art"
	"github.com/silent9669/tsunagi/internal/tui"
	"github.com/silent9669/tsunagi/internal/ui"
)

// Screen events the App routes on. Each screen emits only its own types.
type menuPickMsg struct{ value string }
type providersDoneMsg struct{}
type providersBackMsg struct{}
type skillsDoneMsg struct{}
type skillsBackMsg struct{}
type wizPickMsg struct{ value string } // welcome: continue|skip; ready: start|menu

type wizSkipMsg struct{} // esc during a wizard step

// menuScreen wraps tui.Menu and reports picks upward.
type menuScreen struct {
	opts tui.Options
	th   tui.Theme
	menu tui.Menu
	app  *App
	w, h int
}

func newMenuScreen(o tui.Options, th tui.Theme, items []tui.Item, initial int) *menuScreen {
	return &menuScreen{opts: o, th: th, menu: tui.NewMenu(items, initial, th, o.UTF8), w: o.W, h: o.H}
}

func (m *menuScreen) Init() tea.Cmd { return nil }

func (m *menuScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.menu.Update(msg)
	m.menu = nm.(tui.Menu)
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = v.Width, v.Height
	case tui.Chosen:
		return m, func() tea.Msg { return menuPickMsg{value: v.Value} }
	case tui.Quit:
		return m, func() tea.Msg { return quitAppMsg{} }
	}
	return m, cmd
}

func (m *menuScreen) View() tea.View {
	return m.menu.View()
}

// introScreen plays the splash: the eel swims, the wordmark dissolves in and
// menu rows appear one by one. Any key skips to the end.
type introScreen struct {
	opts    tui.Options
	th      tui.Theme
	app     *App
	start   time.Time
	skipped bool
	done    bool
	w, h    int
}

func newIntroScreen(o tui.Options, th tui.Theme, a *App) *introScreen {
	return &introScreen{opts: o, th: th, app: a, start: time.Now(), w: o.W, h: o.H}
}

func (s *introScreen) skip() { s.skipped = true }

func (s *introScreen) Init() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (s *introScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = v.Width, v.Height
	case tea.KeyPressMsg:
		if !s.done {
			s.done = true
			return s, func() tea.Msg { return introDone{} }
		}
	case tickMsg:
		el := time.Since(s.start)
		if s.skipped || el > 1600*time.Millisecond {
			s.done = true
			return s, func() tea.Msg { return introDone{} }
		}
		return s, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
	}
	return s, nil
}

func (s *introScreen) View() tea.View {
	el := time.Since(s.start)
	if s.skipped {
		el = 2 * time.Second
	}
	t := float64(el) / float64(700*time.Millisecond)
	if t > 1 {
		t = 1
	}
	var b strings.Builder
	// wordmark dissolves in
	logo := ui.LogoLines()
	maxW := 0
	for _, l := range logo {
		if n := tui.WidthOf(l); n > maxW {
			maxW = n
		}
	}
	if maxW < 1 {
		maxW = 1
	}
	// build cell grids: blank -> logo
	rows := len(logo)
	cols := maxW
	blank := make([][]art.Cell, rows)
	full := make([][]art.Cell, rows)
	for i, l := range logo {
		blank[i] = make([]art.Cell, cols)
		full[i] = make([]art.Cell, cols)
		x := 0
		for _, r := range l {
			if r != ' ' {
				full[i][x] = art.Cell{R: r, On: true}
			}
			x++
		}
	}
	mixed := art.Dissolve(blank, full, t)
	for _, row := range mixed {
		var line strings.Builder
		pad := (s.w - cols) / 2
		if pad > 0 {
			line.WriteString(strings.Repeat(" ", pad))
		}
		for _, c := range row {
			if c.On {
				line.WriteRune(c.R)
			} else {
				line.WriteByte(' ')
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	// eel
	artH := s.h - 8 - 6 - 2
	if artH < 4 {
		artH = 4
	}
	if artH > 10 {
		artH = 10
	}
	ao := s.opts
	ao.H = artH
	phase := float64(el) / float64(time.Second) * 2
	b.WriteString(tui.EelArt(ao, s.th, phase))
	b.WriteByte('\n')
	b.WriteByte('\n')
	// menu rows appear staggered
	items := s.app.menuItems()
	for i, it := range items {
		showAt := 900*time.Millisecond + time.Duration(i)*60*time.Millisecond
		if el < showAt && !s.skipped {
			continue
		}
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		line := prefix + it.Label
		if it.Tag != "" {
			pad := s.w - tui.WidthOf(line) - tui.WidthOf(it.Tag) - 1
			if pad < 1 {
				pad = 1
			}
			line += strings.Repeat(" ", pad) + s.th.Dim.Render(it.Tag)
		}
		if i == 0 {
			b.WriteString(s.th.Selected.Render(line))
		} else {
			b.WriteString(s.th.Text.Render(line))
		}
		b.WriteByte('\n')
	}
	return tea.NewView(tui.Place(s.w, s.h, b.String()))
}

// wizardWelcome is step 1: eel + one line + Continue/Skip.
type wizardWelcome struct {
	opts tui.Options
	th   tui.Theme
	app  *App
	menu tui.Menu
}

func newWizardWelcome(a *App) *wizardWelcome {
	th := a.theme
	items := []tui.Item{
		{Label: "Continue", Value: "continue"},
		{Label: "Skip setup", Value: "skip"},
	}
	return &wizardWelcome{opts: a.opts, th: th, app: a, menu: tui.NewMenu(items, 0, th, a.opts.UTF8)}
}

func (w *wizardWelcome) Init() tea.Cmd { return nil }

func (w *wizardWelcome) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := w.menu.Update(msg)
	w.menu = nm.(tui.Menu)
	switch msg.(type) {
	case tui.Chosen:
		return w, func() tea.Msg { return wizPickMsg{value: msg.(tui.Chosen).Value} }
	case tui.Quit:
		return w, func() tea.Msg { return wizSkipMsg{} }
	}
	return w, cmd
}

func (w *wizardWelcome) View() tea.View {
	return tea.NewView(w.th.Text.Render("Set up tsunagi") + "\n\n" + w.menu.View().Content)
}

// wizardReady is the last wizard step.
type wizardReady struct {
	opts tui.Options
	th   tui.Theme
	app  *App
	menu tui.Menu
}

func newWizardReady(a *App) *wizardReady {
	items := []tui.Item{
		{Label: "Start Claude Code", Value: "start", Disabled: a.snap.ClaudePath == ""},
		{Label: "Back to menu", Value: "menu"},
	}
	return &wizardReady{opts: a.opts, th: a.theme, app: a, menu: tui.NewMenu(items, 0, a.theme, a.opts.UTF8)}
}

func (r *wizardReady) Init() tea.Cmd { return nil }

func (r *wizardReady) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := r.menu.Update(msg)
	r.menu = nm.(tui.Menu)
	switch msg.(type) {
	case tui.Chosen:
		return r, func() tea.Msg { return wizPickMsg{value: msg.(tui.Chosen).Value} }
	case tui.Quit:
		return r, func() tea.Msg { return wizSkipMsg{} }
	}
	return r, cmd
}

func (r *wizardReady) View() tea.View {
	return tea.NewView(r.menu.View().Content)
}
