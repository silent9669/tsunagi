package app

import (
	"time"

	"charm.land/bubbletea/v2"

	"github.com/silent9669/tsunagi/internal/tui"
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

// View shows the menu rows appearing one by one while the sea above plays.
func (s *introScreen) View() tea.View {
	el := time.Since(s.start)
	items := s.app.menuItems()
	n := len(items)
	if !s.skipped {
		n = 0
		for i := range items {
			if el >= 600*time.Millisecond+time.Duration(i)*70*time.Millisecond {
				n = i + 1
			}
		}
	}
	if n == 0 {
		return tea.NewView("")
	}
	m := fit(tui.NewMenu(items[:n], 0, s.th, s.opts.UTF8), s.w, s.h).(tui.Menu)
	return m.View()
}

func (s *introScreen) legend() []string { return nil }

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

func (w *wizardWelcome) legend() []string {
	return []string{"↑↓|move", "↵|select", "1-2|jump", "esc|skip"}
}

func (r *wizardReady) legend() []string {
	return []string{"↑↓|move", "↵|select", "1-2|jump", "esc|skip"}
}

func (m *menuScreen) legend() []string {
	return []string{"↑↓|move", "↵|select", "1-9|jump", "q|quit"}
}

func (w *wizardWelcome) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v, ok := msg.(tea.MouseClickMsg); ok {
		mo := tea.Mouse(v)
		if mo.Y -= 2; mo.Y < 0 {
			return w, nil
		}
		msg = tea.MouseClickMsg(mo)
	}
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
